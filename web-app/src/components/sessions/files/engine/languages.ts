import type { Extension } from "@codemirror/state";

export async function loadCodemirrorLang(lang: string): Promise<Extension | null> {
  switch (lang) {
    case "javascript":
    case "jsx":
    case "typescript":
    case "tsx": {
      const { javascript } = await import("@codemirror/lang-javascript");
      const isTs = lang === "typescript" || lang === "tsx";
      const isJsx = lang === "jsx" || lang === "tsx";
      return javascript({ typescript: isTs, jsx: isJsx });
    }
    case "python": {
      const { python } = await import("@codemirror/lang-python");
      return python();
    }
    case "go": {
      const { go } = await import("@codemirror/lang-go");
      return go();
    }
    case "markdown": {
      const { markdown } = await import("@codemirror/lang-markdown");
      return markdown();
    }
    case "json": {
      const { json } = await import("@codemirror/lang-json");
      return json();
    }
    case "html": {
      const { html } = await import("@codemirror/lang-html");
      return html();
    }
    case "css":
    case "scss": {
      const { css } = await import("@codemirror/lang-css");
      return css();
    }
    case "rust": {
      const { rust } = await import("@codemirror/lang-rust");
      return rust();
    }
    case "java": {
      const { java } = await import("@codemirror/lang-java");
      return java();
    }
    default:
      return null;
  }
}
