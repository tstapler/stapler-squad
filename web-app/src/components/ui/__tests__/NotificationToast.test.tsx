import fs from "fs";
import path from "path";

describe("NotificationToast", () => {
  it("notification_toast_should_contain_no_timer_effect_when_grepped", () => {
    const source = fs.readFileSync(
      path.join(process.cwd(), "src/components/ui/NotificationToast.tsx"),
      "utf8",
    );
    expect(source).not.toMatch(/setTimeout|setInterval|requestAnimationFrame/);
  });
});
