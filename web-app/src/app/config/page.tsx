// analytics-exempt
import { Redirect } from "@/lib/nav";

export default function ConfigEditorPage() {
  return <Redirect to="/settings?tab=config-files" />;
}
