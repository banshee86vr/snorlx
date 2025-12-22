/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#e8f8f1',
          100: '#d0f0e2',
          200: '#a1e2c5',
          300: '#72d3a8',
          400: '#43c58b',
          500: '#34CB6F',
          600: '#2db865',
          700: '#269a54',
          800: '#1f7d43',
          900: '#186032',
          950: '#0c2818',
        },
        secondary: {
          50: '#e8ecf9',
          100: '#d0d9f2',
          200: '#a1b3e5',
          300: '#728dd8',
          400: '#4367cb',
          500: '#1D4095',
          600: '#1a3881',
          700: '#17306d',
          800: '#142859',
          900: '#112045',
          950: '#0a1023',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },
      keyframes: {
        shimmer: {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(100%)' },
        },
        float1: {
          '0%, 100%': { 
            transform: 'translate(0, 0) scale(1)',
          },
          '33%': { 
            transform: 'translate(30px, -40px) scale(1.1)',
          },
          '66%': { 
            transform: 'translate(-20px, 30px) scale(0.9)',
          },
        },
        float2: {
          '0%, 100%': { 
            transform: 'translate(0, 0) scale(1)',
          },
          '33%': { 
            transform: 'translate(-40px, 30px) scale(0.9)',
          },
          '66%': { 
            transform: 'translate(25px, -35px) scale(1.1)',
          },
        },
        float3: {
          '0%, 100%': { 
            transform: 'translate(0, 0) scale(1)',
          },
          '33%': { 
            transform: 'translate(20px, 40px) scale(1.05)',
          },
          '66%': { 
            transform: 'translate(-30px, -20px) scale(0.95)',
          },
        },
      },
      animation: {
        shimmer: 'shimmer 2s infinite',
        'float-1': 'float1 20s ease-in-out infinite',
        'float-2': 'float2 25s ease-in-out infinite',
        'float-3': 'float3 30s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}
