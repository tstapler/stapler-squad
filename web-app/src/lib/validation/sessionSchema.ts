import { z } from "zod";

export const sessionSchema = z.object({
  title: z
    .string()
    .min(1, "Session title is required")
    .max(100, "Session title must be less than 100 characters")
    .regex(
      /^[a-zA-Z0-9\s\-_]+$/,
      "Title can only contain letters, numbers, spaces, hyphens, and underscores"
    ),

  path: z
    .string()
    .min(1, "Repository path is required")
    .refine(
      (path) => path.startsWith("/") || path.startsWith("~"),
      "Path must be an absolute path (start with / or ~)"
    ),

  workingDir: z
    .string()
    .optional()
    .refine(
      (dir) => !dir || !dir.startsWith("/"),
      "Working directory should be relative to repository root"
    ),

  branch: z
    .string()
    .optional()
    .refine(
      (branch) => !branch || /^[a-zA-Z0-9\-_/]+$/.test(branch),
      "Branch name contains invalid characters"
    ),

  program: z
    .string()
    .optional()
    .refine(
      (prog) => !prog || prog !== "custom",
      "Please enter a custom command"
    ),

  category: z
    .string()
    .max(50, "Category name must be less than 50 characters")
    .optional(),

  prompt: z
    .string()
    .max(1000, "Prompt must be less than 1000 characters")
    .optional(),

  autoYes: z.boolean().optional(),

  existingWorktree: z.string().optional(),

  sessionType: z.enum(["directory", "new_worktree", "existing_worktree"]).optional(),

  useTitleAsBranch: z.boolean().optional(),
}).refine(
  (data) => {
    // If sessionType is existing_worktree, existingWorktree path is required
    if (data.sessionType === "existing_worktree") {
      return !!data.existingWorktree && data.existingWorktree.length > 0;
    }
    return true;
  },
  {
    message: "Existing worktree path is required when using existing worktree",
    path: ["existingWorktree"],
  }
).refine(
  (data) => {
    // If sessionType is new_worktree and not using title as branch, branch is required
    if (data.sessionType === "new_worktree" && !data.useTitleAsBranch) {
      return !!data.branch && data.branch.length > 0;
    }
    return true;
  },
  {
    message: "Branch name is required when creating new worktree",
    path: ["branch"],
  }
);

export type SessionFormData = z.infer<typeof sessionSchema>;

export const defaultValues: Partial<SessionFormData> = {
  program: "claude",
  autoYes: false,
  title: "",
  path: "",
  workingDir: "",
  branch: "",
  category: "",
  prompt: "",
  existingWorktree: "",
  sessionType: "new_worktree",
  useTitleAsBranch: true,
};
