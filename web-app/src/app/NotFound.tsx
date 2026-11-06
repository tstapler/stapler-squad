// analytics-exempt
import { Link, usePathname, useRouter } from "@/lib/nav";
import { actions, container, goBack, homeLink, message, pathText, title } from "./NotFound.css";

/** Shown inside the shell for an unknown URL; the URL stays as typed and the status stays 200 (Go serves index.html). */
export function NotFound() {
  const pathname = usePathname();
  const router = useRouter();
  // With a single entry Back would leave the app, so only the home link shows.
  const canGoBack = typeof window !== "undefined" && window.history.length > 1;
  return (
    <section className={container}>
      <h1 className={title}>Page not found</h1>
      <p className={message}>
        There is no page at <code className={pathText}>{pathname}</code>.
      </p>
      <div className={actions}>
        <Link href="/" className={homeLink}>
          Go to Sessions
        </Link>
        {canGoBack && (
          // analytics-exempt
          <button type="button" className={goBack} onClick={() => router.back()}>
            Go back
          </button>
        )}
      </div>
    </section>
  );
}
