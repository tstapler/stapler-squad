import { redirect } from "next/navigation";

export default function ConfigEditorPage() {
  redirect("/settings?tab=config-files");
}
