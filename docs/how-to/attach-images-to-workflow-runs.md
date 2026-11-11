# Attach images to a workflow run

A workflow whose `command` or `input_template` contains `{{input}}` opens a Run dialog on the Workflows page instead of firing at once. Type the input, add photos or drawings (drop them, or **Choose images**; on a phone this opens the camera or photo picker), then **Run**. Workflows without `{{input}}` still run immediately.

What happens:

- Each image uploads as soon as it is picked to `POST /api/v1/upload-workflow-attachment` and is saved under `<config dir>/workflow-attachments/` (`~/.stapler-squad/workflow-attachments/` by default; see `docs/reference/state-isolation.md` for other instances). The handler is the backlog uploader (`server/services/backlog_attachment_upload_handler.go`): PNG, JPEG, GIF or WebP checked by magic bytes, 10 MB each, no SVG.
- `RunWorkflow` takes `attachment_paths` (max 10). It rejects the call with `InvalidArgument` unless every path is absolute, resolves (after symlinks) to a regular image file inside the workflow attachment directory.
- The session's first prompt ends with `Reference images (read each with your Read tool before you start):` and one path per line, so the agent can open them on its first turn. Without attachments the prompt is unchanged.

Not covered: the omnibar `@slug` launch path and cron fires pass no attachments. Uploaded files are not deleted when a run ends.
