import { vanillaExtract } from "../ve-plugin";
const shim = (name: string, file: string) => ({ name, setup(b: any) { b.onResolve({ filter: new RegExp(`^${name}$`) }, () => ({ path: import.meta.dir + "/../shims/" + file })); } });
export default {
  name: "app-plugins",
  setup(b: any) {
    for (const p of [shim("next/navigation","navigation.tsx"), shim("next/link","link.tsx"), shim("next/dynamic","dynamic.tsx"), shim("next/font/local","font.ts"), shim("next/dist/client/web-vitals","webvitals.ts"), vanillaExtract({ dev: true })]) p.setup(b);
  },
};
