package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"snorlx/backend/internal/models"

	gh "github.com/google/go-github/v92/github"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

// The live poller keeps workflow runs current on the server so browsers never poll GitHub.
//
// Two passes run for the users who are watching: a WebSocket connection whose page is visible, or
// (for API clients and browsers whose socket is down) an authenticated API request within
// watcherWindow. The active pass refreshes every queued or in-progress run and its jobs, the
// discovery pass lists the newest runs of every visible repository to pick up runs started since
// the last sync. Every GitHub request is conditional (If-None-Match), so a resource that did not
// change answers 304, costs no rate-limit budget and triggers no storage write. Changes are stored
// and pushed to the viewers of the repository over the WebSocket hub, exactly like a webhook
// delivery. A user whose page becomes visible wakes both loops so the view is fresh at once.

const (
	// watcherWindow is how long an authenticated API request keeps a user "watching" when the
	// browser has no WebSocket. Fallback polling runs every 10 to 30 seconds, well inside it.
	watcherWindow = 45 * time.Second
	// wakeSpacing is the shortest gap between a timed pass and a pass triggered by a page that
	// became visible, so toggling tabs cannot hammer GitHub.
	wakeSpacing = 5 * time.Second
	// discoveryFactor: new runs are discovered every discoveryFactor active-run intervals.
	discoveryFactor = 6
	// discoveryPerPage is how many of the newest runs a discovery pass lists per repository.
	discoveryPerPage = 20
	// discoveryRepoPage caps the repositories scanned per watching user.
	discoveryRepoPage = 500
	// discoveryWorkers bounds concurrent GitHub requests during a discovery pass.
	discoveryWorkers = 4
	// activePassTimeout and discoveryPassTimeout bound one pass; the next pass starts after the
	// interval anyway, so a slow GitHub never piles up passes.
	activePassTimeout    = 60 * time.Second
	discoveryPassTimeout = 4 * time.Minute
	// userRefreshTimeout bounds the on-demand refresh behind GET /api/pipelines/active?refresh=true.
	userRefreshTimeout = 25 * time.Second
)

// runJobsEvent is the payload of a workflow_job WebSocket event: enough for a client to refetch
// the jobs of exactly one run.
type runJobsEvent struct {
	RunID       int   `json:"run_id"`
	RunGitHubID int64 `json:"run_github_id"`
}

// ===== Watchers (presence) =====

// watchers remembers when each user last made an authenticated API request.
type watchers struct {
	mu   sync.Mutex
	seen map[int]time.Time
}

func newWatchers() *watchers {
	return &watchers{seen: make(map[int]time.Time)}
}

func (w *watchers) touch(userID int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seen[userID] = time.Now()
}

// active returns the users seen after cutoff and forgets the others.
func (w *watchers) active(cutoff time.Time) []int {
	w.mu.Lock()
	defer w.mu.Unlock()
	ids := make([]int, 0, len(w.seen))
	for id, at := range w.seen {
		if at.Before(cutoff) {
			delete(w.seen, id)
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// ===== ETag cache =====

// etagCache stores the last ETag seen per polled GitHub resource.
type etagCache struct {
	mu    sync.Mutex
	etags map[string]string
}

func newETagCache() *etagCache {
	return &etagCache{etags: make(map[string]string)}
}

func (c *etagCache) get(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.etags[key]
}

// set remembers etag for key. An empty etag forgets the key, so the next request for that
// resource is unconditional (GitHub sent no ETag, or the ETag does not cover the whole resource).
func (c *etagCache) set(key, etag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if etag == "" {
		delete(c.etags, key)
		return
	}
	c.etags[key] = etag
}

// retainRuns drops the run and job entries that are not in keep, so completed runs do not
// accumulate. Repository entries are bounded by the number of repositories and stay.
func (c *etagCache) retainRuns(keep map[string]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.etags {
		if !strings.HasPrefix(key, "run:") && !strings.HasPrefix(key, "jobs:") {
			continue
		}
		if _, ok := keep[key]; !ok {
			delete(c.etags, key)
		}
	}
}

func runETagKey(githubID int64) string  { return fmt.Sprintf("run:%d", githubID) }
func jobsETagKey(githubID int64) string { return fmt.Sprintf("jobs:%d", githubID) }
func repoETagKey(repoID int) string     { return fmt.Sprintf("repo:%d", repoID) }

// ===== Per-pass GitHub clients =====

// passClients builds at most one GitHub client per user during a pass and remembers which tokens
// failed: for the whole pass (missing, rejected or rate limited) or for one resource (GitHub
// answered 403 or 404, typically a user whose repository access was revoked after they synced
// it). A watcher with a dead grant must never block the refresh for watchers whose token works.
type passClients struct {
	h       *Handler
	ctx     context.Context
	mu      sync.Mutex
	clients map[int]*gh.Client
	failed  map[int]bool
	// failedFor maps a scope (one run with its jobs, or one repository; see runETagKey and
	// repoETagKey) to the users whose token GitHub refused for it.
	failedFor map[string]map[int]bool
}

func newPassClients(ctx context.Context, h *Handler) *passClients {
	return &passClients{
		h:         h,
		ctx:       ctx,
		clients:   make(map[int]*gh.Client),
		failed:    make(map[int]bool),
		failedFor: make(map[string]map[int]bool),
	}
}

// forUsers returns a usable client for the first of userIDs whose token has not failed in this
// pass, neither globally nor for scope.
func (p *passClients) forUsers(userIDs []int, scope string) (*gh.Client, int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range userIDs {
		if p.failed[id] || p.failedFor[scope][id] {
			continue
		}
		if client, ok := p.clients[id]; ok {
			return client, id, true
		}
		user, err := p.h.storage.GetUserByID(p.ctx, id)
		if err != nil || user == nil || user.AccessToken == "" {
			p.failed[id] = true
			continue
		}
		client := p.h.ghClient.GetUserClient(p.ctx, &oauth2.Token{AccessToken: user.AccessToken})
		if client == nil {
			p.failed[id] = true
			continue
		}
		p.clients[id] = client
		return client, id, true
	}
	return nil, 0, false
}

func (p *passClients) markFailed(userID int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failed[userID] = true
	delete(p.clients, userID)
}

func (p *passClients) markFailedFor(userID int, scope string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failedFor[scope] == nil {
		p.failedFor[scope] = make(map[int]bool)
	}
	p.failedFor[scope][userID] = true
}

// withClient runs request with the token of each candidate user in turn until one succeeds for
// scope. It returns false when no candidate is left or the failure is transient; in that case the
// resource waits for the next pass.
func (h *Handler) withClient(clients *passClients, users []int, scope, what, repo string, request func(client *gh.Client, userID int) error) bool {
	for {
		client, userID, ok := clients.forUsers(users, scope)
		if !ok {
			return false
		}
		err := request(client, userID)
		if err == nil {
			return true
		}
		if !h.notePollError(clients, userID, scope, err, what, repo) {
			return false
		}
	}
}

// ===== Loop =====

// RunLivePoller refreshes runs for watching users until ctx is cancelled. Active runs are checked
// every interval; new runs are discovered every discoveryFactor intervals.
func (h *Handler) RunLivePoller(ctx context.Context, interval time.Duration) {
	go h.pollLoop(ctx, interval*discoveryFactor, h.wakeDiscovery, h.discoverNewRuns)
	h.pollLoop(ctx, interval, h.wakeActive, h.refreshActiveRuns)
}

// pollLoop runs pass immediately, then every `every`, and early when wake fires (unless a pass ran
// within wakeSpacing). Passes never overlap.
func (h *Handler) pollLoop(ctx context.Context, every time.Duration, wake <-chan struct{}, pass func(context.Context, []int)) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	var lastPass time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-wake:
			if time.Since(lastPass) < wakeSpacing {
				continue
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		if users := h.watchingUsers(); len(users) > 0 {
			pass(ctx, users)
			lastPass = time.Now()
		}
		timer.Reset(every)
	}
}

// UserActivated is called by the WebSocket hub when a user's page becomes visible. Both loops run
// a pass as soon as they are idle; repeated signals coalesce.
func (h *Handler) UserActivated(int) {
	for _, wake := range []chan struct{}{h.wakeActive, h.wakeDiscovery} {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// noteAPIActivity records an authenticated request as presence. A browser whose WebSocket reports
// page visibility is tracked through that instead, so its API traffic must not keep the poller
// alive after the page was hidden. API token clients and browsers without such a socket (socket
// down, or a client that predates presence frames) are tracked through their requests.
func (h *Handler) noteAPIActivity(userID int, bearer bool) {
	if !bearer && h.wsHub != nil && h.wsHub.ReportsPresence(userID) {
		return
	}
	h.watchers.touch(userID)
}

// watchingUsers is the union of users with a visible page on the WebSocket and users seen on the
// API recently.
func (h *Handler) watchingUsers() []int {
	ids := make(map[int]struct{})
	if h.wsHub != nil {
		for _, id := range h.wsHub.ActiveUserIDs() {
			ids[id] = struct{}{}
		}
	}
	for _, id := range h.watchers.active(time.Now().Add(-watcherWindow)) {
		ids[id] = struct{}{}
	}
	out := make([]int, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// refreshUserRuns is the on-demand variant behind ?refresh=true: both passes, for one user, bounded
// by a request-sized timeout. Shared ETags make it cheap when the background poller is running.
func (h *Handler) refreshUserRuns(ctx context.Context, userID int) {
	ctx, cancel := context.WithTimeout(ctx, userRefreshTimeout)
	defer cancel()
	users := []int{userID}
	h.refreshActiveRuns(ctx, users)
	h.discoverNewRuns(ctx, users)
}

// ===== Active pass =====

type activeTarget struct {
	run   models.WorkflowRun
	users []int
}

// refreshActiveRuns re-reads every queued or in-progress run visible to users from GitHub.
func (h *Handler) refreshActiveRuns(ctx context.Context, users []int) {
	ctx, cancel := context.WithTimeout(ctx, activePassTimeout)
	defer cancel()

	targets := make(map[int]*activeTarget)
	var order []int
	for _, userID := range users {
		runs, err := h.storage.ListActivePipelines(ctx, userID)
		if err != nil {
			log.Warn().Err(err).Int("user_id", userID).Msg("Live poller: failed to list active runs")
			continue
		}
		for i := range runs {
			target, ok := targets[runs[i].ID]
			if !ok {
				target = &activeTarget{run: runs[i]}
				targets[runs[i].ID] = target
				order = append(order, runs[i].ID)
			}
			target.users = append(target.users, userID)
		}
	}

	keep := make(map[string]struct{}, 2*len(order))
	for _, id := range order {
		target := targets[id]
		keep[runETagKey(target.run.GitHubID)] = struct{}{}
		keep[jobsETagKey(target.run.GitHubID)] = struct{}{}
	}
	h.etags.retainRuns(keep)

	clients := newPassClients(ctx, h)
	for _, id := range order {
		if ctx.Err() != nil {
			return
		}
		target := targets[id]
		h.refreshActiveRun(ctx, clients, &target.run, target.users)
	}
}

// refreshActiveRun checks one run and its jobs with conditional requests and pushes changes.
func (h *Handler) refreshActiveRun(ctx context.Context, clients *passClients, run *models.WorkflowRun, users []int) {
	fullName, ok := h.runRepoFullName(ctx, run)
	if !ok {
		return
	}
	owner, repoName, ok := splitFullName(fullName)
	if !ok {
		return
	}

	// An ETag is remembered only after the matching storage write succeeded. Otherwise a failed
	// write would be followed by 304 answers and the run would stay wrong until GitHub changed it.
	runKey := runETagKey(run.GitHubID)
	var (
		ghRun   *gh.WorkflowRun
		etag    string
		changed bool
	)
	ok = h.withClient(clients, users, runKey, "run", fullName, func(client *gh.Client, _ int) error {
		var err error
		ghRun, etag, changed, err = h.ghClient.GetWorkflowRunIfChanged(ctx, client, owner, repoName, run.GitHubID, h.etags.get(runKey))
		return err
	})
	if !ok {
		return
	}
	if changed {
		before := stateOf(run)
		saved, err := h.storage.UpsertRun(ctx, h.applyGitHubRun(run, ghRun))
		if err != nil {
			log.Error().Err(err).Int64("run_github_id", run.GitHubID).Msg("Live poller: failed to save run")
			return
		}
		h.etags.set(runKey, etag)
		if before != stateOf(saved) {
			h.wsHub.SendWorkflowRunUpdate(h.repoViewers(ctx, run.RepoID), saved)
		}
	}

	// Same scope as the run itself: a token that cannot see the run cannot see its jobs.
	jobsKey := jobsETagKey(run.GitHubID)
	var ghJobs []*gh.WorkflowJob
	ok = h.withClient(clients, users, runKey, "jobs", fullName, func(client *gh.Client, _ int) error {
		var err error
		ghJobs, etag, changed, err = h.ghClient.ListWorkflowJobsIfChanged(ctx, client, owner, repoName, run.GitHubID, h.etags.get(jobsKey))
		return err
	})
	if !ok || !changed {
		return
	}
	for _, ghJob := range ghJobs {
		if _, err := h.storage.UpsertJob(ctx, h.convertWorkflowJob(ghJob, run.ID)); err != nil {
			log.Error().Err(err).Int64("job_id", ghJob.GetID()).Msg("Live poller: failed to save job")
			return
		}
	}
	h.etags.set(jobsKey, etag)
	if len(ghJobs) > 0 {
		h.wsHub.SendWorkflowJobUpdate(h.repoViewers(ctx, run.RepoID), runJobsEvent{RunID: run.ID, RunGitHubID: run.GitHubID})
	}
}

// runRepoFullName prefers the repository joined by ListActivePipelines and falls back to storage.
func (h *Handler) runRepoFullName(ctx context.Context, run *models.WorkflowRun) (string, bool) {
	if run.Repository != nil && run.Repository.FullName != "" {
		return run.Repository.FullName, true
	}
	repo, err := h.storage.GetRepository(ctx, run.RepoID)
	if err != nil {
		return "", false
	}
	return repo.FullName, true
}

// ===== Discovery pass =====

type repoTarget struct {
	repo  models.Repository
	users []int
}

// discoverNewRuns lists the newest runs of every repository visible to users so runs started since
// the last sync appear without a manual action.
func (h *Handler) discoverNewRuns(ctx context.Context, users []int) {
	ctx, cancel := context.WithTimeout(ctx, discoveryPassTimeout)
	defer cancel()

	targets := make(map[int]*repoTarget)
	var order []int
	for _, userID := range users {
		repos, _, err := h.storage.ListRepositories(ctx, userID, 1, discoveryRepoPage, "")
		if err != nil {
			log.Warn().Err(err).Int("user_id", userID).Msg("Live poller: failed to list repositories")
			continue
		}
		for i := range repos {
			target, ok := targets[repos[i].ID]
			if !ok {
				target = &repoTarget{repo: repos[i]}
				targets[repos[i].ID] = target
				order = append(order, repos[i].ID)
			}
			target.users = append(target.users, userID)
		}
	}

	clients := newPassClients(ctx, h)
	sem := make(chan struct{}, discoveryWorkers)
	var wg sync.WaitGroup
	for _, id := range order {
		if ctx.Err() != nil {
			break
		}
		target := targets[id]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			h.discoverRepoRuns(ctx, clients, target.repo, target.users)
		}()
	}
	wg.Wait()
}

// discoverRepoRuns lists the newest runs of one repository and stores the ones that are new or
// changed. One workflow_run event per repository tells viewers to refetch.
func (h *Handler) discoverRepoRuns(ctx context.Context, clients *passClients, repo models.Repository, users []int) {
	owner, repoName, ok := splitFullName(repo.FullName)
	if !ok {
		return
	}

	key := repoETagKey(repo.ID)
	var (
		ghRuns  []*gh.WorkflowRun
		etag    string
		changed bool
		userID  int
	)
	ok = h.withClient(clients, users, key, "repository runs", repo.FullName, func(client *gh.Client, id int) error {
		var err error
		ghRuns, etag, changed, err = h.ghClient.ListRecentWorkflowRunsIfChanged(ctx, client, owner, repoName, discoveryPerPage, h.etags.get(key))
		userID = id
		return err
	})
	if !ok || !changed {
		return
	}

	// The ETag is remembered only once every listed run is stored; a failed write or a missing
	// workflow index must make the next pass list the repository again.
	stored, err := h.storage.ListWorkflows(ctx, userID, &repo.ID)
	if err != nil {
		log.Warn().Err(err).Str("repo", repo.FullName).Msg("Live poller: failed to list workflows")
		return
	}
	workflows := indexWorkflows(stored)

	var latestChange *models.WorkflowRun
	allStored := true
	for _, ghRun := range ghRuns {
		run, ok := h.runFromGitHub(ghRun, repo.ID, workflows)
		if !ok {
			continue
		}
		before, known := h.storedState(ctx, run.GitHubID)
		saved, err := h.storage.UpsertRun(ctx, run)
		if err != nil {
			log.Warn().Err(err).Int64("run_github_id", run.GitHubID).Msg("Live poller: failed to save discovered run")
			allStored = false
			continue
		}
		if latestChange == nil && (!known || before != stateOf(saved)) {
			latestChange = saved
		}
	}
	if allStored {
		h.etags.set(key, etag)
	}
	if latestChange != nil {
		h.wsHub.SendWorkflowRunUpdate(h.repoViewers(ctx, repo.ID), latestChange)
	}
}

// storedState snapshots the status of a run before an upsert, which the memory storage applies in
// place. known is false when the run was not stored yet.
func (h *Handler) storedState(ctx context.Context, githubID int64) (runState, bool) {
	previous, err := h.storage.GetRunByGitHubID(ctx, githubID)
	if err != nil || previous == nil {
		return runState{}, false
	}
	return stateOf(previous), true
}

// runState is the part of a run that viewers see change.
type runState struct {
	status     string
	conclusion string
}

func stateOf(run *models.WorkflowRun) runState {
	state := runState{status: run.Status}
	if run.Conclusion != nil {
		state.conclusion = *run.Conclusion
	}
	return state
}

// notePollError logs a GitHub failure and decides whether another watcher's token should be tried
// for the same scope. A rejected or rate-limited token rests for the whole pass; a 403 or 404
// means this user can no longer see the resource (access revoked after sync, or a deleted run), so
// only that scope is retried with the next user. Anything else is transient and waits for the
// next pass.
func (h *Handler) notePollError(clients *passClients, userID int, scope string, err error, what, repo string) (tryNextUser bool) {
	var rateErr *gh.RateLimitError
	var abuseErr *gh.AbuseRateLimitError
	switch status := gitHubStatus(err); {
	case errors.As(err, &rateErr), errors.As(err, &abuseErr):
		clients.markFailed(userID)
		log.Warn().Err(err).Int("user_id", userID).Msg("Live poller: GitHub rate limit reached, pausing this token until the next pass")
		return true
	case status == http.StatusUnauthorized:
		clients.markFailed(userID)
		log.Warn().Int("user_id", userID).Msg("Live poller: GitHub rejected the stored token")
		return true
	case status == http.StatusForbidden, status == http.StatusNotFound:
		clients.markFailedFor(userID, scope)
		log.Debug().Int("user_id", userID).Int("status", status).Str("what", what).Str("repo", repo).Msg("Live poller: user cannot read this resource, trying another watcher")
		return true
	default:
		log.Debug().Err(err).Str("what", what).Str("repo", repo).Str("scope", scope).Msg("Live poller: GitHub request failed")
		return false
	}
}
