import daisyui from "daisyui";
export default {
  content: ["./index.html", "./src/**/*.{vue,ts}"],
  plugins: [daisyui],
  daisyui: { themes: ["light", "dark"], logs: false },
};
