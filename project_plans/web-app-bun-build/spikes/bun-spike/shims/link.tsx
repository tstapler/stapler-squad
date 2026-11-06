import React from "react";
export default function Link({ href, children, ...rest }: any) { return <a href={typeof href === "string" ? href : href?.pathname} {...rest}>{children}</a>; }
