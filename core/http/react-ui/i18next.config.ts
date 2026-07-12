import { defineConfig } from 'i18next-cli';

export default defineConfig({
  "locales": [
    "en",
    "it",
    "es",
    "de",
    "zh-CN",
    "id"
  ],
  "extract": {
    "input": [
      "src/**/*.{js,jsx}"
    ],
    "output": "public/locales/{{language}}/{{namespace}}.json",
    "defaultNS": "common",
    "keySeparator": ".",
    "nsSeparator": ":",
    "functions": [
      "t",
      "*.t"
    ],
    "transComponents": [
      "Trans"
    ]
  },
  "types": {
    "input": [
      "locales/{{language}}/{{namespace}}.json"
    ],
    "output": "src/types/i18next.d.ts"
  }
});