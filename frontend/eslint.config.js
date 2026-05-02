import tsPlugin from "@typescript-eslint/eslint-plugin";
import reactHooks from "eslint-plugin-react-hooks";

export default [
  ...tsPlugin.configs["flat/recommended"],
  {
    files: ["src/**/*.ts", "src/**/*.tsx"],
    plugins: {
      "react-hooks": reactHooks,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // React Compiler-style rules added in 7.1+ reject common valid patterns
      // (layout sync, stable handlers in useMemo, auth bootstrap). Keep 7.1.x for
      // compatibility but relax these until refactors land.
      "react-hooks/refs": "off",
      "react-hooks/set-state-in-effect": "off",
    },
  },
];
