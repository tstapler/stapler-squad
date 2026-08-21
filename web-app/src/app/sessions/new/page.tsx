"use client";
// analytics-exempt

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, Suspense } from "react";

function RedirectContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  useEffect(() => {
    const duplicate = searchParams.get("duplicate");
    if (duplicate) {
      router.replace(`/?duplicate=${duplicate}`);
    } else {
      router.replace("/?new=true");
    }
  }, [router, searchParams]);

  return <div>Redirecting...</div>;
}

export default function NewSessionPage() {
  return (
    <Suspense fallback={<div>Redirecting...</div>}>
      <RedirectContent />
    </Suspense>
  );
}
