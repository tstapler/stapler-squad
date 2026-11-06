import React, { lazy, Suspense } from "react";
export default function dynamic(loader: any, opts: any = {}) {
  const L = lazy(async () => { const m = await loader(); return { default: m.default ?? m }; });
  return (props: any) => <Suspense fallback={opts.loading ? React.createElement(opts.loading) : null}><L {...props} /></Suspense>;
}
