"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2513],{

/***/ 2663:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ useAbortableRequest)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useAbortableRequest auto */ 
/**
 * Returns a `start()` function that hands back an AbortSignal for the next
 * request, automatically aborting whatever request this hook last started
 * (on the next call) and on unmount.
 *
 * A hook/component tied to a fast-changing key (session id, file path, ...)
 * that fires a request per render/effect without cancellation leaves every
 * prior request's promise — and everything it closes over — alive until it
 * resolves or times out. That's what happened in useSessionVcs: switching
 * sessions rapidly with no cancellation took the tab from ~44MB to ~154MB of
 * JS heap over 138 switches (measured 2026-09-04), each abandoned request
 * living until its own `deadline_exceeded`.
 *
 * Usage — one instance per independent request stream (a hook fetching two
 * unrelated things, like status and diff, uses two):
 *
 *   const startStatus = useAbortableRequest();
 *   const fetchStatus = useCallback(async () => {
 *     const signal = startStatus();
 *     try {
 *       const res = await client.getVCSStatus({ id }, { signal });
 *       if (signal.aborted) return;
 *       setStatus(res.vcsStatus);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id, startStatus]);
 *
 * Every state update after the await must still be guarded by
 * `signal.aborted` — this hook only owns the controller's lifecycle, not
 * your component's state. The require-abort-signal ESLint rule
 * (web-app/eslint-plugin-rpc-lifecycle) enforces that every RPC client call
 * inside lib/hooks/** and lib/contexts/** passes a `{ signal }` at all.
 */ function useAbortableRequest() {
    const controllerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const start = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        var _controllerRef_current;
        (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        const controller = new AbortController();
        controllerRef.current = controller;
        return controller.signal;
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            var _controllerRef_current;
            return (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        };
    }, []);
    return start;
}


/***/ }),

/***/ 11538:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GM: () => (/* binding */ parseGitHubRef),
/* harmony export */   Pu: () => (/* binding */ isGitHubRef),
/* harmony export */   p2: () => (/* binding */ getRepoFullName)
/* harmony export */ });
/* unused harmony exports RefType, getCloneUrl, getHtmlUrl, getDisplayName, getSuggestedSessionName */
/**
 * GitHub URL Parser - TypeScript implementation mirroring github/url_parser.go
 * Parses GitHub URLs and shorthand references into structured data
 */ /**
 * Types of GitHub references that can be parsed
 */ var RefType = /*#__PURE__*/ function(RefType) {
    RefType["PR"] = "PR";
    RefType["Branch"] = "Branch";
    RefType["Repo"] = "Repository";
    RefType["File"] = "File";
    RefType["Commit"] = "Commit";
    RefType["Issue"] = "Issue";
    RefType["Compare"] = "Compare";
    RefType["Release"] = "Release";
    return RefType;
}({});
const GITHUB_COM = "github.com";
function escapeRegExp(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
function buildHostURLPatterns(host) {
    const h = escapeRegExp(host);
    return {
        // https://<host>/owner/repo/pull/123
        prURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+)/pull/(\\d+)(?:/.*)?$")),
        // https://<host>/owner/repo/tree/branch-name
        branchURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+)/tree/(.+)$")),
        // https://<host>/owner/repo/blob/branch/path/to/file.go#L10-L20
        fileURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+)/blob/([^/]+)/(.+?)(?:#L(\\d+)(?:-L(\\d+))?)?$")),
        // https://<host>/owner/repo/commit/abc123def456
        commitURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+)/commit/([a-fA-F0-9]+)(?:/.*)?$")),
        // https://<host>/owner/repo/issues/42
        issueURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+)/issues/(\\d+)(?:/.*)?$")),
        // https://<host>/owner/repo/compare/main...feature
        compareURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+)/compare/([^.]+)\\.\\.\\.(.+)$")),
        // https://<host>/owner/repo/releases/tag/v1.0.0
        releaseURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+)/releases/tag/(.+)$")),
        // https://<host>/owner/repo or <host>/owner/repo
        repoURLPattern: new RegExp("^(?:https?://)?".concat(h, "/([^/]+)/([^/]+?)(?:\\.git)?/?$")),
        // git@<host>:owner/repo.git
        sshURLPattern: new RegExp("^git@".concat(h, ":([^/]+)/([^/]+?)(?:\\.git)?$")),
        // ssh://git@<host>/owner/repo.git
        sshProtocolURLPattern: new RegExp("^ssh://git@".concat(h, "/([^/]+)/([^/]+?)(?:\\.git)?$"))
    };
}
const hostPatternCache = new Map();
function patternsForHost(host) {
    let patterns = hostPatternCache.get(host);
    if (!patterns) {
        patterns = buildHostURLPatterns(host);
        hostPatternCache.set(host, patterns);
    }
    return patterns;
}
// Shorthand patterns
// owner/repo:branch
const shorthandBranchPattern = /^([^/:]+)\/([^/:]+):(.+)$/;
// owner/repo (no colon, no https)
const shorthandRepoPattern = /^([^/:]+)\/([^/:]+)$/;
/**
 * Checks if a string could be a valid GitHub username or repo name
 * GitHub names can contain alphanumeric, hyphens, underscores, and dots
 * They can't start with a hyphen
 */ function isValidGitHubName(name) {
    if (!name || name.length > 100) {
        return false;
    }
    // Can't start with a hyphen
    if (name[0] === '-') {
        return false;
    }
    // Must contain only valid characters
    for (const char of name){
        const code = char.charCodeAt(0);
        const isLowerCase = code >= 97 && code <= 122; // a-z
        const isUpperCase = code >= 65 && code <= 90; // A-Z
        const isDigit = code >= 48 && code <= 57; // 0-9
        const isSpecial = char === '-' || char === '_' || char === '.';
        if (!isLowerCase && !isUpperCase && !isDigit && !isSpecial) {
            return false;
        }
    }
    return true;
}
/**
 * Parses a GitHub URL or shorthand reference into a ParsedGitHubRef
 *
 * Supported formats:
 * - https://github.com/owner/repo/pull/123
 * - https://github.com/owner/repo/tree/branch-name
 * - https://github.com/owner/repo/blob/branch/path/to/file.go
 * - https://github.com/owner/repo/blob/branch/file.go#L10 (with line number)
 * - https://github.com/owner/repo/blob/branch/file.go#L10-L20 (with line range)
 * - https://github.com/owner/repo/commit/abc123
 * - https://github.com/owner/repo/issues/42
 * - https://github.com/owner/repo/compare/main...feature
 * - https://github.com/owner/repo/releases/tag/v1.0.0
 * - https://github.com/owner/repo
 * - github.com/owner/repo
 * - git@github.com:owner/repo.git (SSH)
 * - ssh://git@github.com/owner/repo (SSH protocol)
 * - owner/repo:branch-name
 * - owner/repo
 *
 * Tries every URL-based pattern (PR, file, commit, issue, compare, release,
 * branch, repo, SSH) against a single host. Shorthand forms (owner/repo) have
 * no host component and are handled by the caller once, not per-host.
 */ function parseGitHubRefForHost(trimmed, host) {
    const p = patternsForHost(host);
    const hostField = host === GITHUB_COM ? {} : {
        host
    };
    const prMatch = trimmed.match(p.prURLPattern);
    if (prMatch) {
        const prNum = parseInt(prMatch[3], 10);
        if (isNaN(prNum)) return null;
        return {
            type: "PR",
            owner: prMatch[1],
            repo: prMatch[2],
            prNumber: prNum,
            originalUrl: trimmed,
            ...hostField
        };
    }
    // File/blob URL must be tried before branch URL since it's more specific
    const fileMatch = trimmed.match(p.fileURLPattern);
    if (fileMatch) {
        const ref = {
            type: "File",
            owner: fileMatch[1],
            repo: fileMatch[2],
            branch: fileMatch[3],
            filePath: fileMatch[4],
            originalUrl: trimmed,
            ...hostField
        };
        if (fileMatch[5]) ref.lineStart = parseInt(fileMatch[5], 10);
        if (fileMatch[6]) ref.lineEnd = parseInt(fileMatch[6], 10);
        return ref;
    }
    const commitMatch = trimmed.match(p.commitURLPattern);
    if (commitMatch) {
        return {
            type: "Commit",
            owner: commitMatch[1],
            repo: commitMatch[2],
            commitSHA: commitMatch[3],
            originalUrl: trimmed,
            ...hostField
        };
    }
    const issueMatch = trimmed.match(p.issueURLPattern);
    if (issueMatch) {
        const issueNum = parseInt(issueMatch[3], 10);
        if (isNaN(issueNum)) return null;
        return {
            type: "Issue",
            owner: issueMatch[1],
            repo: issueMatch[2],
            issueNumber: issueNum,
            originalUrl: trimmed,
            ...hostField
        };
    }
    const compareMatch = trimmed.match(p.compareURLPattern);
    if (compareMatch) {
        return {
            type: "Compare",
            owner: compareMatch[1],
            repo: compareMatch[2],
            baseBranch: compareMatch[3],
            headBranch: compareMatch[4],
            originalUrl: trimmed,
            ...hostField
        };
    }
    const releaseMatch = trimmed.match(p.releaseURLPattern);
    if (releaseMatch) {
        return {
            type: "Release",
            owner: releaseMatch[1],
            repo: releaseMatch[2],
            tag: releaseMatch[3],
            originalUrl: trimmed,
            ...hostField
        };
    }
    const branchMatch = trimmed.match(p.branchURLPattern);
    if (branchMatch) {
        return {
            type: "Branch",
            owner: branchMatch[1],
            repo: branchMatch[2],
            branch: branchMatch[3],
            originalUrl: trimmed,
            ...hostField
        };
    }
    const repoMatch = trimmed.match(p.repoURLPattern);
    if (repoMatch) {
        return {
            type: "Repository",
            owner: repoMatch[1],
            repo: repoMatch[2],
            originalUrl: trimmed,
            ...hostField
        };
    }
    // git@<host>:owner/repo.git
    const sshMatch = trimmed.match(p.sshURLPattern);
    if (sshMatch) {
        return {
            type: "Repository",
            owner: sshMatch[1],
            repo: sshMatch[2],
            originalUrl: trimmed,
            ...hostField
        };
    }
    // ssh://git@<host>/owner/repo.git
    const sshProtocolMatch = trimmed.match(p.sshProtocolURLPattern);
    if (sshProtocolMatch) {
        return {
            type: "Repository",
            owner: sshProtocolMatch[1],
            repo: sshProtocolMatch[2],
            originalUrl: trimmed,
            ...hostField
        };
    }
    return null;
}
/**
 * @param input - GitHub URL or shorthand reference
 * @param enterpriseHosts - Configured GitHub Enterprise hostnames (e.g. from
 *   useGitHubEnterpriseHosts()) to recognize in addition to github.com.
 * @returns Parsed reference or null if invalid
 */ function parseGitHubRef(input) {
    let enterpriseHosts = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : [];
    const trimmed = input.trim();
    if (!trimmed) {
        return null;
    }
    const hosts = [
        GITHUB_COM,
        ...enterpriseHosts.filter(Boolean)
    ];
    for (const host of hosts){
        const ref = parseGitHubRefForHost(trimmed, host);
        if (ref) return ref;
    }
    // Try shorthand with branch (owner/repo:branch)
    const shorthandBranchMatch = trimmed.match(shorthandBranchPattern);
    if (shorthandBranchMatch) {
        return {
            type: "Branch",
            owner: shorthandBranchMatch[1],
            repo: shorthandBranchMatch[2],
            branch: shorthandBranchMatch[3],
            originalUrl: trimmed
        };
    }
    // Try shorthand repo (owner/repo)
    // Only match if it looks like a GitHub reference (contains exactly one slash)
    const shorthandRepoMatch = trimmed.match(shorthandRepoPattern);
    if (shorthandRepoMatch) {
        const owner = shorthandRepoMatch[1];
        const repo = shorthandRepoMatch[2];
        if (isValidGitHubName(owner) && isValidGitHubName(repo)) {
            return {
                type: "Repository",
                owner,
                repo,
                originalUrl: trimmed
            };
        }
    }
    return null;
}
/**
 * Quick check if the input string looks like a GitHub URL or reference
 * This is a fast check that doesn't validate the full format
 *
 * @param input - String to check
 * @param enterpriseHosts - Configured GitHub Enterprise hostnames (e.g. from
 *   useGitHubEnterpriseHosts()) to recognize in addition to github.com.
 * @returns true if it looks like a GitHub reference
 */ function isGitHubRef(input) {
    let enterpriseHosts = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : [];
    const trimmed = input.trim();
    if (!trimmed) {
        return false;
    }
    const hosts = [
        GITHUB_COM,
        ...enterpriseHosts.filter(Boolean)
    ];
    for (const host of hosts){
        // Check for explicit HTTPS URLs
        if (trimmed.includes("".concat(host, "/"))) {
            return true;
        }
        // Check for SSH URL format (git@<host>:owner/repo)
        if (trimmed.startsWith("git@".concat(host, ":"))) {
            return true;
        }
        // Check for SSH protocol URL (ssh://git@<host>/)
        if (trimmed.startsWith("ssh://git@".concat(host, "/"))) {
            return true;
        }
    }
    // Check for shorthand formats (owner/repo or owner/repo:branch)
    // Must have exactly one "/" and optionally one ":"
    const slashCount = (trimmed.match(/\//g) || []).length;
    if (slashCount === 1) {
        const parts = trimmed.split('/', 2);
        if (parts.length === 2 && isValidGitHubName(parts[0])) {
            // Remove any :branch suffix for repo name validation
            const repo = parts[1].split(':', 2)[0];
            if (isValidGitHubName(repo)) {
                return true;
            }
        }
    }
    return false;
}
/**
 * Get the HTTPS clone URL for the repository
 * @param ref - Parsed GitHub reference
 * @param host - GitHub(.com) or GitHub Enterprise host; defaults to ref.host
 *   (set by parseGitHubRef when the ref came from a GHE URL) or "github.com".
 * @returns Clone URL (e.g., "https://github.com/owner/repo.git")
 */ function getCloneUrl(ref) {
    let host = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : ref.host || GITHUB_COM;
    return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, ".git");
}
/**
 * Get the human-readable GitHub URL
 * @param ref - Parsed GitHub reference
 * @param host - GitHub(.com) or GitHub Enterprise host; defaults to ref.host
 *   (set by parseGitHubRef when the ref came from a GHE URL) or "github.com".
 * @returns HTML URL for viewing in browser
 */ function getHtmlUrl(ref) {
    let host = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : ref.host || GITHUB_COM;
    switch(ref.type){
        case "PR":
            return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, "/pull/").concat(ref.prNumber);
        case "Branch":
            return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, "/tree/").concat(ref.branch);
        case "File":
            {
                let url = "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, "/blob/").concat(ref.branch, "/").concat(ref.filePath);
                if (ref.lineStart) {
                    if (ref.lineEnd && ref.lineEnd !== ref.lineStart) {
                        url += "#L".concat(ref.lineStart, "-L").concat(ref.lineEnd);
                    } else {
                        url += "#L".concat(ref.lineStart);
                    }
                }
                return url;
            }
        case "Commit":
            return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, "/commit/").concat(ref.commitSHA);
        case "Issue":
            return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, "/issues/").concat(ref.issueNumber);
        case "Compare":
            return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, "/compare/").concat(ref.baseBranch, "...").concat(ref.headBranch);
        case "Release":
            return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo, "/releases/tag/").concat(ref.tag);
        default:
            return "https://".concat(host, "/").concat(ref.owner, "/").concat(ref.repo);
    }
}
/**
 * Get a human-readable name for the reference
 * @param ref - Parsed GitHub reference
 * @returns Display name (e.g., "owner/repo#123", "owner/repo:branch", "owner/repo")
 */ function getDisplayName(ref) {
    switch(ref.type){
        case "PR":
            return "".concat(ref.owner, "/").concat(ref.repo, "#").concat(ref.prNumber);
        case "Branch":
            return "".concat(ref.owner, "/").concat(ref.repo, ":").concat(ref.branch);
        case "File":
            {
                let name = "".concat(ref.owner, "/").concat(ref.repo, ":").concat(ref.branch, "/").concat(ref.filePath);
                if (ref.lineStart) {
                    if (ref.lineEnd && ref.lineEnd !== ref.lineStart) {
                        name += "#L".concat(ref.lineStart, "-L").concat(ref.lineEnd);
                    } else {
                        name += "#L".concat(ref.lineStart);
                    }
                }
                return name;
            }
        case "Commit":
            {
                let shortSHA = ref.commitSHA || '';
                if (shortSHA.length > 7) {
                    shortSHA = shortSHA.substring(0, 7);
                }
                return "".concat(ref.owner, "/").concat(ref.repo, "@").concat(shortSHA);
            }
        case "Issue":
            return "".concat(ref.owner, "/").concat(ref.repo, "#").concat(ref.issueNumber, " (issue)");
        case "Compare":
            return "".concat(ref.owner, "/").concat(ref.repo, ": ").concat(ref.baseBranch, "...").concat(ref.headBranch);
        case "Release":
            return "".concat(ref.owner, "/").concat(ref.repo, "@").concat(ref.tag);
        default:
            return "".concat(ref.owner, "/").concat(ref.repo);
    }
}
/**
 * Get a suggested session name based on the reference
 * @param ref - Parsed GitHub reference
 * @returns Suggested session name
 */ function getSuggestedSessionName(ref) {
    switch(ref.type){
        case "PR":
            return "".concat(ref.owner, "-").concat(ref.repo, "-pr-").concat(ref.prNumber);
        case "Branch":
            {
                // Sanitize branch name for session name
                const branch = (ref.branch || '').replace(/\//g, '-');
                return "".concat(ref.owner, "-").concat(ref.repo, "-").concat(branch);
            }
        case "File":
            {
                // Use the file name as part of the session name
                const parts = (ref.filePath || '').split('/');
                let fileName = parts[parts.length - 1];
                // Remove extension for cleaner name
                const extIndex = fileName.lastIndexOf('.');
                if (extIndex > 0) {
                    fileName = fileName.substring(0, extIndex);
                }
                const branch = (ref.branch || '').replace(/\//g, '-');
                return "".concat(ref.owner, "-").concat(ref.repo, "-").concat(branch, "-").concat(fileName);
            }
        case "Commit":
            {
                let shortSHA = ref.commitSHA || '';
                if (shortSHA.length > 7) {
                    shortSHA = shortSHA.substring(0, 7);
                }
                return "".concat(ref.owner, "-").concat(ref.repo, "-commit-").concat(shortSHA);
            }
        case "Issue":
            return "".concat(ref.owner, "-").concat(ref.repo, "-issue-").concat(ref.issueNumber);
        case "Compare":
            {
                const base = (ref.baseBranch || '').replace(/\//g, '-');
                const head = (ref.headBranch || '').replace(/\//g, '-');
                return "".concat(ref.owner, "-").concat(ref.repo, "-").concat(base, "-vs-").concat(head);
            }
        case "Release":
            {
                const tag = (ref.tag || '').replace(/\//g, '-');
                return "".concat(ref.owner, "-").concat(ref.repo, "-").concat(tag);
            }
        default:
            return "".concat(ref.owner, "-").concat(ref.repo);
    }
}
/**
 * Get the repository full name in "owner/repo" format
 * @param ref - Parsed GitHub reference
 * @returns Repository full name
 */ function getRepoFullName(ref) {
    return "".concat(ref.owner, "/").concat(ref.repo);
}


/***/ }),

/***/ 13798:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $Y: () => (/* binding */ AddGitHubAccountWithTokenRequestSchema),
/* harmony export */   AY: () => (/* binding */ ClosePRDirectRequestSchema),
/* harmony export */   Al: () => (/* binding */ ListGitHubAccountsRequestSchema),
/* harmony export */   Ex: () => (/* binding */ PRDraftToggleRequestSchema),
/* harmony export */   Gs: () => (/* binding */ WatchUserPRsRequestSchema),
/* harmony export */   LV: () => (/* binding */ RevokeGitHubTokenRequestSchema),
/* harmony export */   Rx: () => (/* binding */ ListGitHubCLIHostsRequestSchema),
/* harmony export */   SH: () => (/* binding */ StartGitHubDeviceAuthRequestSchema),
/* harmony export */   Tn: () => (/* binding */ NudgeSessionForPRRequestSchema),
/* harmony export */   UX: () => (/* binding */ NudgeOutcome),
/* harmony export */   WJ: () => (/* binding */ MergePRDirectRequestSchema),
/* harmony export */   X_: () => (/* binding */ DeviceAuthStatus),
/* harmony export */   aI: () => (/* binding */ GitHubUserService),
/* harmony export */   it: () => (/* binding */ AddGitHubAccountFromCLIRequestSchema),
/* harmony export */   m$: () => (/* binding */ AccountPollState),
/* harmony export */   nQ: () => (/* binding */ PollGitHubDeviceAuthRequestSchema),
/* harmony export */   xE: () => (/* binding */ PRKeySchema)
/* harmony export */ });
/* unused harmony exports file_session_v1_github_user, PRActionResponseSchema, GitHubAccountSchema, GitHubAuthStateSchema, ListUserPRsRequestSchema, ListUserPRsResponseSchema, AccountPollStatusSchema, UserPREventSchema, GetGitHubAuthStateRequestSchema, GetGitHubAuthStateResponseSchema, StartGitHubDeviceAuthResponseSchema, PollGitHubDeviceAuthResponseSchema, RevokeGitHubTokenResponseSchema, ListGitHubAccountsResponseSchema, AddGitHubAccountWithTokenResponseSchema, GitHubCLIHostSchema, ListGitHubCLIHostsResponseSchema, NudgeSessionForPRResponseSchema, AccountPollStateSchema, DeviceAuthStatusSchema, NudgeOutcomeSchema, NudgeReason, NudgeReasonSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(1844);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(15033);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(95765);
/* harmony import */ var _types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/github_user.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/github_user.proto.
 */ const file_session_v1_github_user = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .fileDesc */ .w)("ChxzZXNzaW9uL3YxL2dpdGh1Yl91c2VyLnByb3RvEgpzZXNzaW9uLnYxInEKFE1lcmdlUFJEaXJlY3RSZXF1ZXN0Eg0KBW93bmVyGAEgASgJEgwKBHJlcG8YAiABKAkSDgoGbnVtYmVyGAMgASgFEgwKBGhvc3QYBCABKAkSEwoGbWV0aG9kGAUgASgJSACIAQFCCQoHX21ldGhvZCJRChRDbG9zZVBSRGlyZWN0UmVxdWVzdBINCgVvd25lchgBIAEoCRIMCgRyZXBvGAIgASgJEg4KBm51bWJlchgDIAEoBRIMCgRob3N0GAQgASgJIlEKFFBSRHJhZnRUb2dnbGVSZXF1ZXN0Eg0KBW93bmVyGAEgASgJEgwKBHJlcG8YAiABKAkSDgoGbnVtYmVyGAMgASgFEgwKBGhvc3QYBCABKAkiNAoQUFJBY3Rpb25SZXNwb25zZRIPCgdzdWNjZXNzGAEgASgIEg8KB21lc3NhZ2UYAiABKAkiRQoNR2l0SHViQWNjb3VudBIQCgh1c2VybmFtZRgBIAEoCRIUCgxpc19lbnZfdG9rZW4YAiABKAgSDAoEaG9zdBgDIAEoCSJ6Cg9HaXRIdWJBdXRoU3RhdGUSEQoJYXZhaWxhYmxlGAEgASgIEhAKCHVzZXJuYW1lGAIgASgJEhUKDWVycm9yX21lc3NhZ2UYAyABKAkSKwoIYWNjb3VudHMYBCADKAsyGS5zZXNzaW9uLnYxLkdpdEh1YkFjY291bnQiFAoSTGlzdFVzZXJQUnNSZXF1ZXN0IqABChNMaXN0VXNlclBSc1Jlc3BvbnNlEh8KA3BycxgBIAMoCzISLnNlc3Npb24udjEuVXNlclBSEi8KCmF1dGhfc3RhdGUYAiABKAsyGy5zZXNzaW9uLnYxLkdpdEh1YkF1dGhTdGF0ZRI3ChBhY2NvdW50X3N0YXR1c2VzGAMgAygLMh0uc2Vzc2lvbi52MS5BY2NvdW50UG9sbFN0YXR1cyJ1ChFBY2NvdW50UG9sbFN0YXR1cxIMCgRob3N0GAEgASgJEhUKDWFjY291bnRfbG9naW4YAiABKAkSKwoFc3RhdGUYAyABKA4yHC5zZXNzaW9uLnYxLkFjY291bnRQb2xsU3RhdGUSDgoGZGV0YWlsGAQgASgJIhUKE1dhdGNoVXNlclBSc1JlcXVlc3QirAEKC1VzZXJQUkV2ZW50EhIKCmV2ZW50X3R5cGUYASABKAkSHwoDcHJzGAIgAygLMhIuc2Vzc2lvbi52MS5Vc2VyUFISLwoKYXV0aF9zdGF0ZRgDIAEoCzIbLnNlc3Npb24udjEuR2l0SHViQXV0aFN0YXRlEjcKEGFjY291bnRfc3RhdHVzZXMYBCADKAsyHS5zZXNzaW9uLnYxLkFjY291bnRQb2xsU3RhdHVzIhsKGUdldEdpdEh1YkF1dGhTdGF0ZVJlcXVlc3QiTQoaR2V0R2l0SHViQXV0aFN0YXRlUmVzcG9uc2USLwoKYXV0aF9zdGF0ZRgBIAEoCzIbLnNlc3Npb24udjEuR2l0SHViQXV0aFN0YXRlIiwKHFN0YXJ0R2l0SHViRGV2aWNlQXV0aFJlcXVlc3QSDAoEaG9zdBgBIAEoCSKHAQodU3RhcnRHaXRIdWJEZXZpY2VBdXRoUmVzcG9uc2USEwoLZGV2aWNlX2NvZGUYASABKAkSEQoJdXNlcl9jb2RlGAIgASgJEhgKEHZlcmlmaWNhdGlvbl91cmkYAyABKAkSEgoKZXhwaXJlc19pbhgEIAEoBRIQCghpbnRlcnZhbBgFIAEoBSIyChtQb2xsR2l0SHViRGV2aWNlQXV0aFJlcXVlc3QSEwoLZGV2aWNlX2NvZGUYASABKAkijAEKHFBvbGxHaXRIdWJEZXZpY2VBdXRoUmVzcG9uc2USLAoGc3RhdHVzGAEgASgOMhwuc2Vzc2lvbi52MS5EZXZpY2VBdXRoU3RhdHVzEg0KBWVycm9yGAIgASgJEi8KCmF1dGhfc3RhdGUYAyABKAsyGy5zZXNzaW9uLnYxLkdpdEh1YkF1dGhTdGF0ZSI6ChhSZXZva2VHaXRIdWJUb2tlblJlcXVlc3QSEAoIdXNlcm5hbWUYASABKAkSDAoEaG9zdBgCIAEoCSIbChlSZXZva2VHaXRIdWJUb2tlblJlc3BvbnNlIhsKGUxpc3RHaXRIdWJBY2NvdW50c1JlcXVlc3QiYwoaTGlzdEdpdEh1YkFjY291bnRzUmVzcG9uc2USKwoIYWNjb3VudHMYASADKAsyGS5zZXNzaW9uLnYxLkdpdEh1YkFjY291bnQSGAoQZW50ZXJwcmlzZV9ob3N0cxgCIAMoCSI/CiBBZGRHaXRIdWJBY2NvdW50V2l0aFRva2VuUmVxdWVzdBIMCgRob3N0GAEgASgJEg0KBXRva2VuGAIgASgJIlQKIUFkZEdpdEh1YkFjY291bnRXaXRoVG9rZW5SZXNwb25zZRIvCgphdXRoX3N0YXRlGAEgASgLMhsuc2Vzc2lvbi52MS5HaXRIdWJBdXRoU3RhdGUiRgoNR2l0SHViQ0xJSG9zdBIMCgRob3N0GAEgASgJEhAKCHVzZXJuYW1lGAIgASgJEhUKDWFscmVhZHlfYWRkZWQYAyABKAgiGwoZTGlzdEdpdEh1YkNMSUhvc3RzUmVxdWVzdCJcChpMaXN0R2l0SHViQ0xJSG9zdHNSZXNwb25zZRIoCgVob3N0cxgBIAMoCzIZLnNlc3Npb24udjEuR2l0SHViQ0xJSG9zdBIUCgxnaF9hdmFpbGFibGUYAiABKAgiLgoeQWRkR2l0SHViQWNjb3VudEZyb21DTElSZXF1ZXN0EgwKBGhvc3QYASABKAkiQgoFUFJLZXkSDAoEaG9zdBgBIAEoCRINCgVvd25lchgCIAEoCRIMCgRyZXBvGAMgASgJEg4KBm51bWJlchgEIAEoBSJNChhOdWRnZVNlc3Npb25Gb3JQUlJlcXVlc3QSHQoCcHIYASABKAsyES5zZXNzaW9uLnYxLlBSS2V5EhIKCnNlc3Npb25faWQYAiABKAkilAEKGU51ZGdlU2Vzc2lvbkZvclBSUmVzcG9uc2USKQoHb3V0Y29tZRgBIAEoDjIYLnNlc3Npb24udjEuTnVkZ2VPdXRjb21lEigKB3JlYXNvbnMYAiADKA4yFy5zZXNzaW9uLnYxLk51ZGdlUmVhc29uEhIKCnNlc3Npb25faWQYAyABKAkSDgoGZGV0YWlsGAQgASgJKrkBChBBY2NvdW50UG9sbFN0YXRlEiIKHkFDQ09VTlRfUE9MTF9TVEFURV9VTlNQRUNJRklFRBAAEhkKFUFDQ09VTlRfUE9MTF9TVEFURV9PSxABEiMKH0FDQ09VTlRfUE9MTF9TVEFURV9VTkFVVEhPUklaRUQQAhIjCh9BQ0NPVU5UX1BPTExfU1RBVEVfUkFURV9MSU1JVEVEEAMSHAoYQUNDT1VOVF9QT0xMX1NUQVRFX0VSUk9SEAQqtQEKEERldmljZUF1dGhTdGF0dXMSIgoeREVWSUNFX0FVVEhfU1RBVFVTX1VOU1BFQ0lGSUVEEAASHgoaREVWSUNFX0FVVEhfU1RBVFVTX1BFTkRJTkcQARIfChtERVZJQ0VfQVVUSF9TVEFUVVNfQ09NUExFVEUQAhIeChpERVZJQ0VfQVVUSF9TVEFUVVNfRVhQSVJFRBADEhwKGERFVklDRV9BVVRIX1NUQVRVU19FUlJPUhAEKqACCgxOdWRnZU91dGNvbWUSHQoZTlVER0VfT1VUQ09NRV9VTlNQRUNJRklFRBAAEhsKF05VREdFX09VVENPTUVfREVMSVZFUkVEEAESFgoSTlVER0VfT1VUQ09NRV9CVVNZEAISGAoUTlVER0VfT1VUQ09NRV9QQVVTRUQQAxIbChdOVURHRV9PVVRDT01FX0RVUExJQ0FURRAEEiAKHE5VREdFX09VVENPTUVfTk9USElOR19UT19GSVgQBRIkCiBOVURHRV9PVVRDT01FX1NFU1NJT05fTk9UX0xJTktFRBAGEh4KGk5VREdFX09VVENPTUVfUFJfTk9UX0ZPVU5EEAcSHQoZTlVER0VfT1VUQ09NRV9OT1RfUlVOTklORxAIKpIBCgtOdWRnZVJlYXNvbhIcChhOVURHRV9SRUFTT05fVU5TUEVDSUZJRUQQABIfChtOVURHRV9SRUFTT05fRkFJTElOR19DSEVDS1MQARIjCh9OVURHRV9SRUFTT05fVU5SRVNPTFZFRF9USFJFQURTEAISHwobTlVER0VfUkVBU09OX01FUkdFX0NPTkZMSUNUEAMyzgsKEUdpdEh1YlVzZXJTZXJ2aWNlElAKC0xpc3RVc2VyUFJzEh4uc2Vzc2lvbi52MS5MaXN0VXNlclBSc1JlcXVlc3QaHy5zZXNzaW9uLnYxLkxpc3RVc2VyUFJzUmVzcG9uc2UiABJMCgxXYXRjaFVzZXJQUnMSHy5zZXNzaW9uLnYxLldhdGNoVXNlclBSc1JlcXVlc3QaFy5zZXNzaW9uLnYxLlVzZXJQUkV2ZW50IgAwARJlChJHZXRHaXRIdWJBdXRoU3RhdGUSJS5zZXNzaW9uLnYxLkdldEdpdEh1YkF1dGhTdGF0ZVJlcXVlc3QaJi5zZXNzaW9uLnYxLkdldEdpdEh1YkF1dGhTdGF0ZVJlc3BvbnNlIgASbgoVU3RhcnRHaXRIdWJEZXZpY2VBdXRoEiguc2Vzc2lvbi52MS5TdGFydEdpdEh1YkRldmljZUF1dGhSZXF1ZXN0Gikuc2Vzc2lvbi52MS5TdGFydEdpdEh1YkRldmljZUF1dGhSZXNwb25zZSIAEmsKFFBvbGxHaXRIdWJEZXZpY2VBdXRoEicuc2Vzc2lvbi52MS5Qb2xsR2l0SHViRGV2aWNlQXV0aFJlcXVlc3QaKC5zZXNzaW9uLnYxLlBvbGxHaXRIdWJEZXZpY2VBdXRoUmVzcG9uc2UiABJiChFSZXZva2VHaXRIdWJUb2tlbhIkLnNlc3Npb24udjEuUmV2b2tlR2l0SHViVG9rZW5SZXF1ZXN0GiUuc2Vzc2lvbi52MS5SZXZva2VHaXRIdWJUb2tlblJlc3BvbnNlIgASZQoSTGlzdEdpdEh1YkFjY291bnRzEiUuc2Vzc2lvbi52MS5MaXN0R2l0SHViQWNjb3VudHNSZXF1ZXN0GiYuc2Vzc2lvbi52MS5MaXN0R2l0SHViQWNjb3VudHNSZXNwb25zZSIAEnoKGUFkZEdpdEh1YkFjY291bnRXaXRoVG9rZW4SLC5zZXNzaW9uLnYxLkFkZEdpdEh1YkFjY291bnRXaXRoVG9rZW5SZXF1ZXN0Gi0uc2Vzc2lvbi52MS5BZGRHaXRIdWJBY2NvdW50V2l0aFRva2VuUmVzcG9uc2UiABJlChJMaXN0R2l0SHViQ0xJSG9zdHMSJS5zZXNzaW9uLnYxLkxpc3RHaXRIdWJDTElIb3N0c1JlcXVlc3QaJi5zZXNzaW9uLnYxLkxpc3RHaXRIdWJDTElIb3N0c1Jlc3BvbnNlIgASdgoXQWRkR2l0SHViQWNjb3VudEZyb21DTEkSKi5zZXNzaW9uLnYxLkFkZEdpdEh1YkFjY291bnRGcm9tQ0xJUmVxdWVzdBotLnNlc3Npb24udjEuQWRkR2l0SHViQWNjb3VudFdpdGhUb2tlblJlc3BvbnNlIgASYgoRTnVkZ2VTZXNzaW9uRm9yUFISJC5zZXNzaW9uLnYxLk51ZGdlU2Vzc2lvbkZvclBSUmVxdWVzdBolLnNlc3Npb24udjEuTnVkZ2VTZXNzaW9uRm9yUFJSZXNwb25zZSIAElEKDU1lcmdlUFJEaXJlY3QSIC5zZXNzaW9uLnYxLk1lcmdlUFJEaXJlY3RSZXF1ZXN0Ghwuc2Vzc2lvbi52MS5QUkFjdGlvblJlc3BvbnNlIgASUQoNQ2xvc2VQUkRpcmVjdBIgLnNlc3Npb24udjEuQ2xvc2VQUkRpcmVjdFJlcXVlc3QaHC5zZXNzaW9uLnYxLlBSQWN0aW9uUmVzcG9uc2UiABJUChBDb252ZXJ0UFJUb0RyYWZ0EiAuc2Vzc2lvbi52MS5QUkRyYWZ0VG9nZ2xlUmVxdWVzdBocLnNlc3Npb24udjEuUFJBY3Rpb25SZXNwb25zZSIAEk8KC01hcmtQUlJlYWR5EiAuc2Vzc2lvbi52MS5QUkRyYWZ0VG9nZ2xlUmVxdWVzdBocLnNlc3Npb24udjEuUFJBY3Rpb25SZXNwb25zZSIAQq8BCg5jb20uc2Vzc2lvbi52MUIPR2l0aHViVXNlclByb3RvUAFaQ2dpdGh1Yi5jb20vdHN0YXBsZXIvc3RhcGxlci1zcXVhZC9nZW4vcHJvdG8vZ28vc2Vzc2lvbi92MTtzZXNzaW9udjGiAgNTWFiqAgpTZXNzaW9uLlYxygIKU2Vzc2lvblxWMeICFlNlc3Npb25cVjFcR1BCTWV0YWRhdGHqAgtTZXNzaW9uOjpWMWIGcHJvdG8z", [
    _types_pb__WEBPACK_IMPORTED_MODULE_0__/* .file_session_v1_types */ .m$
]);
/**
 * Describes the message session.v1.MergePRDirectRequest.
 * Use `create(MergePRDirectRequestSchema)` to create a new message.
 */ const MergePRDirectRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 0);
/**
 * Describes the message session.v1.ClosePRDirectRequest.
 * Use `create(ClosePRDirectRequestSchema)` to create a new message.
 */ const ClosePRDirectRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 1);
/**
 * Describes the message session.v1.PRDraftToggleRequest.
 * Use `create(PRDraftToggleRequestSchema)` to create a new message.
 */ const PRDraftToggleRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 2);
/**
 * Describes the message session.v1.PRActionResponse.
 * Use `create(PRActionResponseSchema)` to create a new message.
 */ const PRActionResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 3)));
/**
 * Describes the message session.v1.GitHubAccount.
 * Use `create(GitHubAccountSchema)` to create a new message.
 */ const GitHubAccountSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 4)));
/**
 * Describes the message session.v1.GitHubAuthState.
 * Use `create(GitHubAuthStateSchema)` to create a new message.
 */ const GitHubAuthStateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 5)));
/**
 * Describes the message session.v1.ListUserPRsRequest.
 * Use `create(ListUserPRsRequestSchema)` to create a new message.
 */ const ListUserPRsRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 6)));
/**
 * Describes the message session.v1.ListUserPRsResponse.
 * Use `create(ListUserPRsResponseSchema)` to create a new message.
 */ const ListUserPRsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 7)));
/**
 * Describes the message session.v1.AccountPollStatus.
 * Use `create(AccountPollStatusSchema)` to create a new message.
 */ const AccountPollStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 8)));
/**
 * Describes the message session.v1.WatchUserPRsRequest.
 * Use `create(WatchUserPRsRequestSchema)` to create a new message.
 */ const WatchUserPRsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 9);
/**
 * Describes the message session.v1.UserPREvent.
 * Use `create(UserPREventSchema)` to create a new message.
 */ const UserPREventSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 10)));
/**
 * Describes the message session.v1.GetGitHubAuthStateRequest.
 * Use `create(GetGitHubAuthStateRequestSchema)` to create a new message.
 */ const GetGitHubAuthStateRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 11)));
/**
 * Describes the message session.v1.GetGitHubAuthStateResponse.
 * Use `create(GetGitHubAuthStateResponseSchema)` to create a new message.
 */ const GetGitHubAuthStateResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 12)));
/**
 * Describes the message session.v1.StartGitHubDeviceAuthRequest.
 * Use `create(StartGitHubDeviceAuthRequestSchema)` to create a new message.
 */ const StartGitHubDeviceAuthRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 13);
/**
 * Describes the message session.v1.StartGitHubDeviceAuthResponse.
 * Use `create(StartGitHubDeviceAuthResponseSchema)` to create a new message.
 */ const StartGitHubDeviceAuthResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 14)));
/**
 * Describes the message session.v1.PollGitHubDeviceAuthRequest.
 * Use `create(PollGitHubDeviceAuthRequestSchema)` to create a new message.
 */ const PollGitHubDeviceAuthRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 15);
/**
 * Describes the message session.v1.PollGitHubDeviceAuthResponse.
 * Use `create(PollGitHubDeviceAuthResponseSchema)` to create a new message.
 */ const PollGitHubDeviceAuthResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 16)));
/**
 * Describes the message session.v1.RevokeGitHubTokenRequest.
 * Use `create(RevokeGitHubTokenRequestSchema)` to create a new message.
 */ const RevokeGitHubTokenRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 17);
/**
 * Describes the message session.v1.RevokeGitHubTokenResponse.
 * Use `create(RevokeGitHubTokenResponseSchema)` to create a new message.
 */ const RevokeGitHubTokenResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 18)));
/**
 * Describes the message session.v1.ListGitHubAccountsRequest.
 * Use `create(ListGitHubAccountsRequestSchema)` to create a new message.
 */ const ListGitHubAccountsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 19);
/**
 * Describes the message session.v1.ListGitHubAccountsResponse.
 * Use `create(ListGitHubAccountsResponseSchema)` to create a new message.
 */ const ListGitHubAccountsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 20)));
/**
 * Describes the message session.v1.AddGitHubAccountWithTokenRequest.
 * Use `create(AddGitHubAccountWithTokenRequestSchema)` to create a new message.
 */ const AddGitHubAccountWithTokenRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 21);
/**
 * Describes the message session.v1.AddGitHubAccountWithTokenResponse.
 * Use `create(AddGitHubAccountWithTokenResponseSchema)` to create a new message.
 */ const AddGitHubAccountWithTokenResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 22)));
/**
 * Describes the message session.v1.GitHubCLIHost.
 * Use `create(GitHubCLIHostSchema)` to create a new message.
 */ const GitHubCLIHostSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 23)));
/**
 * Describes the message session.v1.ListGitHubCLIHostsRequest.
 * Use `create(ListGitHubCLIHostsRequestSchema)` to create a new message.
 */ const ListGitHubCLIHostsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 24);
/**
 * Describes the message session.v1.ListGitHubCLIHostsResponse.
 * Use `create(ListGitHubCLIHostsResponseSchema)` to create a new message.
 */ const ListGitHubCLIHostsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 25)));
/**
 * Describes the message session.v1.AddGitHubAccountFromCLIRequest.
 * Use `create(AddGitHubAccountFromCLIRequestSchema)` to create a new message.
 */ const AddGitHubAccountFromCLIRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 26);
/**
 * Describes the message session.v1.PRKey.
 * Use `create(PRKeySchema)` to create a new message.
 */ const PRKeySchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 27);
/**
 * Describes the message session.v1.NudgeSessionForPRRequest.
 * Use `create(NudgeSessionForPRRequestSchema)` to create a new message.
 */ const NudgeSessionForPRRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 28);
/**
 * Describes the message session.v1.NudgeSessionForPRResponse.
 * Use `create(NudgeSessionForPRResponseSchema)` to create a new message.
 */ const NudgeSessionForPRResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 29)));
/**
 * @generated from enum session.v1.AccountPollState
 */ var AccountPollState = /*#__PURE__*/ function(AccountPollState) {
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_UNSPECIFIED = 0;
   */ AccountPollState[AccountPollState["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_OK = 1;
   */ AccountPollState[AccountPollState["OK"] = 1] = "OK";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_UNAUTHORIZED = 2;
   */ AccountPollState[AccountPollState["UNAUTHORIZED"] = 2] = "UNAUTHORIZED";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_RATE_LIMITED = 3;
   */ AccountPollState[AccountPollState["RATE_LIMITED"] = 3] = "RATE_LIMITED";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_ERROR = 4;
   */ AccountPollState[AccountPollState["ERROR"] = 4] = "ERROR";
    return AccountPollState;
}({});
/**
 * Describes the enum session.v1.AccountPollState.
 */ const AccountPollStateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 0)));
/**
 * DeviceAuthStatus describes the outcome of a single poll attempt.
 *
 * @generated from enum session.v1.DeviceAuthStatus
 */ var DeviceAuthStatus = /*#__PURE__*/ function(DeviceAuthStatus) {
    /**
   * @generated from enum value: DEVICE_AUTH_STATUS_UNSPECIFIED = 0;
   */ DeviceAuthStatus[DeviceAuthStatus["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * waiting for user to authorize
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_PENDING = 1;
   */ DeviceAuthStatus[DeviceAuthStatus["PENDING"] = 1] = "PENDING";
    /**
   * token received and stored in keychain
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_COMPLETE = 2;
   */ DeviceAuthStatus[DeviceAuthStatus["COMPLETE"] = 2] = "COMPLETE";
    /**
   * device code expired; restart the flow
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_EXPIRED = 3;
   */ DeviceAuthStatus[DeviceAuthStatus["EXPIRED"] = 3] = "EXPIRED";
    /**
   * unexpected error
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_ERROR = 4;
   */ DeviceAuthStatus[DeviceAuthStatus["ERROR"] = 4] = "ERROR";
    return DeviceAuthStatus;
}({});
/**
 * Describes the enum session.v1.DeviceAuthStatus.
 */ const DeviceAuthStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 1)));
/**
 * NudgeOutcome is the typed result of NudgeSessionForPR.
 *
 * @generated from enum session.v1.NudgeOutcome
 */ var NudgeOutcome = /*#__PURE__*/ function(NudgeOutcome) {
    /**
   * @generated from enum value: NUDGE_OUTCOME_UNSPECIFIED = 0;
   */ NudgeOutcome[NudgeOutcome["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_DELIVERED = 1;
   */ NudgeOutcome[NudgeOutcome["DELIVERED"] = 1] = "DELIVERED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_BUSY = 2;
   */ NudgeOutcome[NudgeOutcome["BUSY"] = 2] = "BUSY";
    /**
   * @generated from enum value: NUDGE_OUTCOME_PAUSED = 3;
   */ NudgeOutcome[NudgeOutcome["PAUSED"] = 3] = "PAUSED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_DUPLICATE = 4;
   */ NudgeOutcome[NudgeOutcome["DUPLICATE"] = 4] = "DUPLICATE";
    /**
   * @generated from enum value: NUDGE_OUTCOME_NOTHING_TO_FIX = 5;
   */ NudgeOutcome[NudgeOutcome["NOTHING_TO_FIX"] = 5] = "NOTHING_TO_FIX";
    /**
   * @generated from enum value: NUDGE_OUTCOME_SESSION_NOT_LINKED = 6;
   */ NudgeOutcome[NudgeOutcome["SESSION_NOT_LINKED"] = 6] = "SESSION_NOT_LINKED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_PR_NOT_FOUND = 7;
   */ NudgeOutcome[NudgeOutcome["PR_NOT_FOUND"] = 7] = "PR_NOT_FOUND";
    /**
   * The linked session no longer exists or is not running (deleted or crashed),
   * as opposed to PAUSED, which Open session can resume.
   *
   * @generated from enum value: NUDGE_OUTCOME_NOT_RUNNING = 8;
   */ NudgeOutcome[NudgeOutcome["NOT_RUNNING"] = 8] = "NOT_RUNNING";
    return NudgeOutcome;
}({});
/**
 * Describes the enum session.v1.NudgeOutcome.
 */ const NudgeOutcomeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 2)));
/**
 * NudgeReason is one kind of problem a nudge prompt covers.
 *
 * @generated from enum session.v1.NudgeReason
 */ var NudgeReason = /*#__PURE__*/ function(NudgeReason) {
    /**
   * @generated from enum value: NUDGE_REASON_UNSPECIFIED = 0;
   */ NudgeReason[NudgeReason["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: NUDGE_REASON_FAILING_CHECKS = 1;
   */ NudgeReason[NudgeReason["FAILING_CHECKS"] = 1] = "FAILING_CHECKS";
    /**
   * @generated from enum value: NUDGE_REASON_UNRESOLVED_THREADS = 2;
   */ NudgeReason[NudgeReason["UNRESOLVED_THREADS"] = 2] = "UNRESOLVED_THREADS";
    /**
   * @generated from enum value: NUDGE_REASON_MERGE_CONFLICT = 3;
   */ NudgeReason[NudgeReason["MERGE_CONFLICT"] = 3] = "MERGE_CONFLICT";
    return NudgeReason;
}({});
/**
 * Describes the enum session.v1.NudgeReason.
 */ const NudgeReasonSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 3)));
/**
 * GitHubUserService provides access to the authenticated user's GitHub pull
 * requests across all repos. Data is served from the in-process UserPRCache
 * (which polls the GitHub GraphQL API in the background). No subprocess calls.
 *
 * @generated from service session.v1.GitHubUserService
 */ const GitHubUserService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .serviceDesc */ .Z)(file_session_v1_github_user, 0);


/***/ }),

/***/ 24031:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ connectApi)
/* harmony export */ });
/* harmony import */ var _reduxjs_toolkit_query_react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(90453);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);


// Generic base query for ConnectRPC unary calls.
// arg shape: { call: () => Promise<unknown> }
const connectBaseQuery = async (param)=>{
    let { call } = param;
    try {
        const result = await call();
        return {
            data: result
        };
    } catch (err) {
        if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T) {
            // Store rawMessage, not message — .message is ConnectRPC's
            // `[code] message`-prefixed string, and callers reading this error
            // slice (e.g. BacklogItemDetail.tsx's lastError?.message reads)
            // display it verbatim to users.
            return {
                error: {
                    status: err.code,
                    error: err.rawMessage || err.message
                }
            };
        }
        const msg = err instanceof Error ? err.message : "Unknown error";
        return {
            error: {
                status: -1,
                error: msg
            }
        };
    }
};
const connectApi = (0,_reduxjs_toolkit_query_react__WEBPACK_IMPORTED_MODULE_1__/* .createApi */ .xP)({
    reducerPath: "connectApi",
    baseQuery: connectBaseQuery,
    tagTypes: [
        "Approvals",
        "GuidanceRequests"
    ],
    endpoints: ()=>({})
});


/***/ }),

/***/ 24621:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   N0: () => (/* binding */ toggleSelection),
/* harmony export */   Ub: () => (/* binding */ selectAll),
/* harmony export */   wr: () => (/* binding */ clearSelection),
/* harmony export */   xc: () => (/* binding */ selectionAfterBulk)
/* harmony export */ });
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(73559);

const initialState = {
    selectedIds: [],
    selectMode: false
};
// Separate from bulkSelectionSlice (sessions) so selecting backlog rows never
// touches session selection.
const backlogBulkSelectionSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_0__/* .createSlice */ .Z0)({
    name: "backlogBulkSelection",
    initialState,
    reducers: {
        toggleSelection (state, action) {
            const idx = state.selectedIds.indexOf(action.payload);
            if (idx >= 0) {
                state.selectedIds.splice(idx, 1);
            } else {
                state.selectedIds.push(action.payload);
            }
            state.selectMode = true;
        },
        selectAll (state, action) {
            state.selectedIds = action.payload;
            state.selectMode = action.payload.length > 0;
        },
        clearSelection (state) {
            state.selectedIds = [];
            state.selectMode = false;
        }
    }
});
const { toggleSelection, selectAll, clearSelection } = backlogBulkSelectionSlice.actions;
/** Action to apply after a bulk fan-out: clear on full success, otherwise keep only the failed ids selected. */ const selectionAfterBulk = (failedIds)=>failedIds.length === 0 ? clearSelection() : selectAll(failedIds);
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (backlogBulkSelectionSlice.reducer);


/***/ }),

/***/ 25337:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  M_: () => (/* binding */ store),
  jL: () => (/* binding */ useAppDispatch),
  GV: () => (/* binding */ useAppSelector)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@reduxjs+toolkit@2.11.2+71d2413d80c012e4/node_modules/@reduxjs/toolkit/dist/redux-toolkit.modern.mjs + 1 modules
var redux_toolkit_modern = __webpack_require__(73559);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-redux@9.2.0+5436edeee5f9f735/node_modules/react-redux/dist/react-redux.mjs
var react_redux = __webpack_require__(55324);
// EXTERNAL MODULE: ./src/lib/store/backlogItemsSlice.ts
var backlogItemsSlice = __webpack_require__(63939);
// EXTERNAL MODULE: ./src/lib/store/backlogBulkSelectionSlice.ts
var backlogBulkSelectionSlice = __webpack_require__(24621);
;// ./src/lib/store/bulkSelectionSlice.ts

const initialState = {
    selectedIds: []
};
const bulkSelectionSlice = (0,redux_toolkit_modern/* createSlice */.Z0)({
    name: 'bulkSelection',
    initialState,
    reducers: {
        toggleSelection (state, action) {
            const id = action.payload;
            const idx = state.selectedIds.indexOf(id);
            if (idx >= 0) {
                state.selectedIds.splice(idx, 1);
            } else {
                state.selectedIds.push(id);
            }
        },
        selectAll (state, action) {
            state.selectedIds = action.payload;
        },
        clearSelection (state) {
            state.selectedIds = [];
        }
    }
});
const { toggleSelection, selectAll, clearSelection } = bulkSelectionSlice.actions;
/* harmony default export */ const store_bulkSelectionSlice = (bulkSelectionSlice.reducer);

// EXTERNAL MODULE: ./src/lib/store/remotesSlice.ts
var remotesSlice = __webpack_require__(81921);
// EXTERNAL MODULE: ./src/lib/store/reviewQueueSlice.ts
var reviewQueueSlice = __webpack_require__(68453);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/lib/api/connectApi.ts
var connectApi = __webpack_require__(24031);
;// ./src/lib/store/store.ts









const store = (0,redux_toolkit_modern/* configureStore */.U1)({
    reducer: {
        backlogItems: backlogItemsSlice/* default */.Ay,
        backlogBulkSelection: backlogBulkSelectionSlice/* default */.Ay,
        bulkSelection: store_bulkSelectionSlice,
        remotes: remotesSlice/* default */.Ay,
        reviewQueue: reviewQueueSlice/* default */.Ay,
        sessions: sessionsSlice/* default */.Ay,
        [connectApi/* connectApi */.X.reducerPath]: connectApi/* connectApi */.X.reducer
    },
    middleware: (getDefaultMiddleware)=>getDefaultMiddleware().concat(connectApi/* connectApi */.X.middleware)
});
// Typed hooks for use throughout the app
const useAppDispatch = react_redux/* useDispatch */.wA.withTypes();
const useAppSelector = react_redux/* useSelector */.d4.withTypes();


/***/ }),

/***/ 44635:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   P: () => (/* binding */ useGitHubEnterpriseHosts)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_github_user_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13798);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(95444);
/* harmony import */ var _lib_hooks_useAbortableEffect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(64343);
/* __next_internal_client_entry_do_not_use__ useGitHubEnterpriseHosts auto */ 





/**
 * Fetches the GHES hostnames configured on the server via ListGitHubAccounts.
 * github.com is always implicitly available and is not included in the result.
 *
 * Exposes refetch() so callers can re-run this on a relevant event (e.g. the
 * omnibar opening — see OmnibarContext, which mirrors this pattern from
 * useLauncherPresets) rather than only once at mount: a GHE account added
 * after the page loaded would otherwise never appear, silently leaving
 * GitHubEnterpriseURLDetector's host list stale for the rest of the session.
 */ function useGitHubEnterpriseHosts() {
    const [hosts, setHosts] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)([]);
    const [fetchTick, setFetchTick] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(0);
    (0,_lib_hooks_useAbortableEffect__WEBPACK_IMPORTED_MODULE_3__/* .useAbortableEffect */ .B)(async (signal)=>{
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_github_user_pb__WEBPACK_IMPORTED_MODULE_2__/* .GitHubUserService */ .aI, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
        try {
            const res = await client.listGitHubAccounts((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_5__/* .create */ .v)(_gen_session_v1_github_user_pb__WEBPACK_IMPORTED_MODULE_2__/* .ListGitHubAccountsRequestSchema */ .Al, {}), {
                signal
            });
            if (signal.aborted) return;
            setHosts(res.enterpriseHosts);
        } catch (e) {
            if (signal.aborted) return;
            setHosts([]);
        }
    }, [
        fetchTick
    ]);
    const refetch = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>setFetchTick((t)=>t + 1), []);
    return {
        hosts,
        refetch
    };
}


/***/ }),

/***/ 60795:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AY: () => (/* binding */ selectSessionsError),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   Hx: () => (/* binding */ selectDetectedStatusMap),
/* harmony export */   Nh: () => (/* binding */ setError),
/* harmony export */   OS: () => (/* binding */ selectSessionsLoading),
/* harmony export */   QA: () => (/* binding */ setErrorCode),
/* harmony export */   Qr: () => (/* binding */ selectSessionsHasLoadedOnce),
/* harmony export */   YI: () => (/* binding */ setSessions),
/* harmony export */   aA: () => (/* binding */ setConnectionState),
/* harmony export */   dF: () => (/* binding */ removeSession),
/* harmony export */   ih: () => (/* binding */ upsertSession),
/* harmony export */   mN: () => (/* binding */ selectConnectionState),
/* harmony export */   od: () => (/* binding */ selectActiveSessionsSortedByUpdatedAt),
/* harmony export */   pu: () => (/* binding */ removeDetectedStatus),
/* harmony export */   r1: () => (/* binding */ setLoading),
/* harmony export */   rt: () => (/* binding */ selectAllSessions)
/* harmony export */ });
/* unused harmony exports selectSessionById, selectSessionIds, selectSessionsTotal, selectSessionsErrorCode */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13009);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);


const sessionsAdapter = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createEntityAdapter */ .pU)({
    selectId: (session)=>session.id
});
const initialState = sessionsAdapter.getInitialState({
    loading: false,
    hasLoadedOnce: false,
    error: null,
    detectedStatusMap: {},
    connectionState: "disconnected",
    deletedIds: {}
});
const sessionsSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "sessions",
    initialState,
    reducers: {
        setSessions (state, action) {
            state.hasLoadedOnce = true;
            const filtered = action.payload.filter((s)=>!state.deletedIds[s.id]);
            // Preserve existing entity references when a session's data is unchanged
            // (same updatedAt) so React.memo on SessionRowWrapper can skip re-rendering
            // rows untouched by this snapshot/poll — mirrors the guard in upsertSession.
            const merged = filtered.map((incoming)=>{
                const existing = state.entities[incoming.id];
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return existing;
                }
                return incoming;
            });
            sessionsAdapter.setAll(state, merged);
        },
        upsertSession (state, action) {
            // Don't resurrect a deleted session via an in-flight update event
            if (!state.deletedIds[action.payload.id]) {
                // Skip no-op upserts: if updatedAt matches the existing record the data
                // hasn't changed and we'd just cause a spurious re-render. This prevents
                // the N-render storm from WatchSessions initial-snapshot events that
                // duplicate sessions already loaded by the preceding listSessions() call.
                const existing = state.entities[action.payload.id];
                const incoming = action.payload;
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return;
                }
                sessionsAdapter.upsertOne(state, action.payload);
                // Sync detectedStatusMap from the session's proto fields
                const session = action.payload;
                if (session.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.ACTIVE) {
                    // Non-active: clear badge unconditionally
                    delete state.detectedStatusMap[session.id];
                } else if (session.detectedStatus !== undefined && session.detectedStatus !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNSPECIFIED) {
                    var _session_detectedContext;
                    // Active with typed detection: update map from proto field
                    state.detectedStatusMap[session.id] = {
                        detectedStatus: session.detectedStatus,
                        detectedContext: (_session_detectedContext = session.detectedContext) !== null && _session_detectedContext !== void 0 ? _session_detectedContext : ""
                    };
                } else {
                    // Active + UNSPECIFIED: clear
                    delete state.detectedStatusMap[session.id];
                }
            }
        },
        removeSession (state, action) {
            sessionsAdapter.removeOne(state, action.payload);
            state.deletedIds[action.payload] = true;
        },
        setLoading (state, action) {
            state.loading = action.payload;
        },
        setError (state, action) {
            state.error = action.payload;
            // A fresh error/clear always invalidates the previously-classified code — callers that
            // need a code alongside the message dispatch setErrorCode immediately after setError.
            state.errorCode = undefined;
        },
        setErrorCode (state, action) {
            state.errorCode = action.payload;
        },
        setConnectionState (state, action) {
            state.connectionState = action.payload;
        },
        removeDetectedStatus (state, action) {
            delete state.detectedStatusMap[action.payload];
        }
    }
});
const { setSessions, upsertSession, removeSession, setLoading, setError, setErrorCode, setConnectionState, removeDetectedStatus } = sessionsSlice.actions;
// Use the adapter's built-in selectors scoped to the sessions slice
const adapterSelectors = sessionsAdapter.getSelectors((state)=>state.sessions);
const selectAllSessions = adapterSelectors.selectAll;
const selectSessionById = adapterSelectors.selectById;
// Memoized selector: active sessions pre-sorted by updatedAt descending.
// Avoids repeated O(n log n) sort inside render functions that show recent sessions.
const selectActiveSessionsSortedByUpdatedAt = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectAllSessions, (sessions)=>sessions.filter((s)=>s.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.UNSPECIFIED).sort((a, b)=>{
        var _b_updatedAt, _a_updatedAt, _b_createdAt, _a_createdAt;
        var _b_updatedAt_seconds, _a_updatedAt_seconds;
        const byUpdated = Number((_b_updatedAt_seconds = (_b_updatedAt = b.updatedAt) === null || _b_updatedAt === void 0 ? void 0 : _b_updatedAt.seconds) !== null && _b_updatedAt_seconds !== void 0 ? _b_updatedAt_seconds : 0) - Number((_a_updatedAt_seconds = (_a_updatedAt = a.updatedAt) === null || _a_updatedAt === void 0 ? void 0 : _a_updatedAt.seconds) !== null && _a_updatedAt_seconds !== void 0 ? _a_updatedAt_seconds : 0);
        if (byUpdated !== 0) return byUpdated;
        var _b_createdAt_seconds, _a_createdAt_seconds;
        const byCreated = Number((_b_createdAt_seconds = (_b_createdAt = b.createdAt) === null || _b_createdAt === void 0 ? void 0 : _b_createdAt.seconds) !== null && _b_createdAt_seconds !== void 0 ? _b_createdAt_seconds : 0) - Number((_a_createdAt_seconds = (_a_createdAt = a.createdAt) === null || _a_createdAt === void 0 ? void 0 : _a_createdAt.seconds) !== null && _a_createdAt_seconds !== void 0 ? _a_createdAt_seconds : 0);
        if (byCreated !== 0) return byCreated;
        return a.id.localeCompare(b.id);
    }));
const selectSessionIds = adapterSelectors.selectIds;
const selectSessionsTotal = adapterSelectors.selectTotal;
const selectSessionsLoading = (state)=>state.sessions.loading;
const selectSessionsHasLoadedOnce = (state)=>state.sessions.hasLoadedOnce;
const selectSessionsError = (state)=>state.sessions.error;
const selectSessionsErrorCode = (state)=>state.sessions.errorCode;
const selectDetectedStatusMap = (state)=>state.sessions.detectedStatusMap;
const selectConnectionState = (state)=>state.sessions.connectionState;
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (sessionsSlice.reducer);


/***/ }),

/***/ 63939:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ removeItem),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   Bi: () => (/* binding */ selectAllBacklogItems),
/* harmony export */   Ky: () => (/* binding */ selectBacklogItemsLiveVersionMap),
/* harmony export */   Se: () => (/* binding */ appendActivityNote),
/* harmony export */   vH: () => (/* binding */ upsertItem),
/* harmony export */   zc: () => (/* binding */ selectBacklogItemById)
/* harmony export */ });
/* unused harmony exports selectBacklogItemsMap, selectBacklogItemsTotal, makeSelectBacklogItemsByStatus */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13009);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(28765);


const initialState = {
    items: {},
    liveVersion: {}
};
/** Returns the epoch-ms value of a proto Timestamp, or 0 if unset (oldest possible). */ function timestampMs(ts) {
    return ts ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_0__/* .timestampDate */ .FP)(ts).getTime() : 0;
}
const backlogItemsSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "backlogItems",
    initialState,
    reducers: {
        /**
     * Upserts a backlog item, guarded by a real `updatedAt`-based staleness
     * check (not sessionsSlice.upsertSession's equal-only check): an incoming
     * item strictly older than the currently-stored item for the same id is
     * dropped, so out-of-order event delivery (e.g. concurrent publishers,
     * stream replay) can never regress the store to older data.
     *
     * `isSnapshot` (default `true`) drives `liveVersion` bookkeeping only —
     * pass `false` exclusively for a genuine live `BacklogItemEvent` (i.e.
     * `event.isSnapshot === false`) so Epic 6.1's flash treatment can tell a
     * real-time change apart from a snapshot/replay/poll refresh.
     */ upsertItem: {
            reducer (state, action) {
                const { item: incoming, isSnapshot } = action.payload;
                const existing = state.items[incoming.id];
                if (existing && timestampMs(incoming.updatedAt) < timestampMs(existing.updatedAt)) {
                    return;
                }
                // Backstop for a partially-loaded event push (item_sessions/activity_notes/
                // allowed_transitions dropped by a publishItemChanged call site or a
                // non-eager-loaded snapshot query) racing a fully-loaded one: never let an
                // empty itemSessions, activityNotes, or allowedTransitions clobber data we
                // already have for this item. A genuine all-removed update would still
                // arrive with a newer updatedAt and populated (even if empty-by-intent)
                // data from a call site that *did* eager-load, so this only masks the
                // known-bad partial-load case, not real removals.
                //
                // activityNotes specifically: every BacklogChangeKind other than
                // ChangeActivityNoteAdded publishes via publishItemChanged ->
                // attachItemSessionsForPublish (session/ent_repository_backlog.go), which
                // re-populates only ItemSessions, never ActivityNotes — so their wire
                // events embed a full protoItem whose activity_notes is empty. Without
                // this guard, the very next status transition/verdict/session-attach would
                // wipe out any notes accumulated via the dedicated appendActivityNote
                // reducer below (ADR-002, Blocker 1 fix).
                //
                // allowedTransitions specifically: a pure function of status, so it's
                // never legitimately empty for a real item — an empty value is always a
                // sparse-DTO/resync artifact (e.g. a stale ListBacklogItems shape).
                //
                // planArtifactsPath specifically: unlike the three fields above, it CAN
                // legitimately go from set back to empty (TransitionBacklogItemStatus
                // resets it when an item moves back to idea/refining, e.g. re-triage) —
                // so this is gated on the item's status being unchanged. Every real
                // reset path changes status in the same write (server-side
                // backlog_service_lifecycle.go), so "same status, plan vanished" is
                // never a legitimate transition and is always a sparse-DTO artifact
                // (ListBacklogItems' backlogItemSummaryToProto omitted this field
                // entirely until the fix alongside this guard — getPrimaryCardAction/
                // itemActions.ts derives a ready item's card action directly from it,
                // so a sparse resync could flip "Approve Plan" to "Trigger Triage").
                let nextItem = incoming;
                if (existing) {
                    var _existing_itemSessions, _incoming_itemSessions, _existing_activityNotes, _incoming_activityNotes, _existing_allowedTransitions, _incoming_allowedTransitions, _existing_blocking, _incoming_blocking;
                    const patch = {};
                    var _existing_itemSessions_length, _incoming_itemSessions_length;
                    if (((_existing_itemSessions_length = (_existing_itemSessions = existing.itemSessions) === null || _existing_itemSessions === void 0 ? void 0 : _existing_itemSessions.length) !== null && _existing_itemSessions_length !== void 0 ? _existing_itemSessions_length : 0) > 0 && ((_incoming_itemSessions_length = (_incoming_itemSessions = incoming.itemSessions) === null || _incoming_itemSessions === void 0 ? void 0 : _incoming_itemSessions.length) !== null && _incoming_itemSessions_length !== void 0 ? _incoming_itemSessions_length : 0) === 0) {
                        patch.itemSessions = existing.itemSessions;
                    }
                    var _existing_activityNotes_length, _incoming_activityNotes_length;
                    if (((_existing_activityNotes_length = (_existing_activityNotes = existing.activityNotes) === null || _existing_activityNotes === void 0 ? void 0 : _existing_activityNotes.length) !== null && _existing_activityNotes_length !== void 0 ? _existing_activityNotes_length : 0) > 0 && ((_incoming_activityNotes_length = (_incoming_activityNotes = incoming.activityNotes) === null || _incoming_activityNotes === void 0 ? void 0 : _incoming_activityNotes.length) !== null && _incoming_activityNotes_length !== void 0 ? _incoming_activityNotes_length : 0) === 0) {
                        patch.activityNotes = existing.activityNotes;
                    }
                    var _existing_allowedTransitions_length, _incoming_allowedTransitions_length;
                    if (((_existing_allowedTransitions_length = (_existing_allowedTransitions = existing.allowedTransitions) === null || _existing_allowedTransitions === void 0 ? void 0 : _existing_allowedTransitions.length) !== null && _existing_allowedTransitions_length !== void 0 ? _existing_allowedTransitions_length : 0) > 0 && ((_incoming_allowedTransitions_length = (_incoming_allowedTransitions = incoming.allowedTransitions) === null || _incoming_allowedTransitions === void 0 ? void 0 : _incoming_allowedTransitions.length) !== null && _incoming_allowedTransitions_length !== void 0 ? _incoming_allowedTransitions_length : 0) === 0) {
                        patch.allowedTransitions = existing.allowedTransitions;
                    }
                    var _existing_blocking_length, _incoming_blocking_length;
                    // blocking is only populated by GetBacklogItem/Add/Remove; Watch
                    // payloads never carry it, so a live event must not erase it.
                    if (((_existing_blocking_length = (_existing_blocking = existing.blocking) === null || _existing_blocking === void 0 ? void 0 : _existing_blocking.length) !== null && _existing_blocking_length !== void 0 ? _existing_blocking_length : 0) > 0 && ((_incoming_blocking_length = (_incoming_blocking = incoming.blocking) === null || _incoming_blocking === void 0 ? void 0 : _incoming_blocking.length) !== null && _incoming_blocking_length !== void 0 ? _incoming_blocking_length : 0) === 0) {
                        patch.blocking = existing.blocking;
                    }
                    if (existing.status === incoming.status && existing.planArtifactsPath && !incoming.planArtifactsPath) {
                        patch.planArtifactsPath = existing.planArtifactsPath;
                    }
                    if (Object.keys(patch).length > 0) {
                        nextItem = {
                            ...incoming,
                            ...patch
                        };
                    }
                }
                state.items[incoming.id] = nextItem;
                if (!isSnapshot) {
                    var _state_liveVersion_incoming_id;
                    state.liveVersion[incoming.id] = ((_state_liveVersion_incoming_id = state.liveVersion[incoming.id]) !== null && _state_liveVersion_incoming_id !== void 0 ? _state_liveVersion_incoming_id : 0) + 1;
                }
            },
            prepare (item) {
                let isSnapshot = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
                return {
                    payload: {
                        item,
                        isSnapshot
                    }
                };
            }
        },
        /** Deletes an item from the map entirely (BacklogItemRemovedEvent — permanent delete, not upsert). */ removeItem (state, action) {
            delete state.items[action.payload];
        },
        /**
     * Targeted single-note append for the dedicated activityNoteAdded live
     * event (ADR-002, backlog-item-activity-log) — appends `note` to
     * `state.items[itemId].activityNotes` without touching any other field,
     * never a call to upsertItem/a wholesale item replace. No-op if the item
     * isn't in the store yet (matches removeItem's "item not present"
     * tolerance) — the next GetBacklogItem/ListBacklogItems refresh will
     * include it.
     */ appendActivityNote (state, action) {
            const { itemId, note } = action.payload;
            const existing = state.items[itemId];
            if (!existing) return;
            var _existing_activityNotes;
            state.items[itemId] = {
                ...existing,
                activityNotes: [
                    ...(_existing_activityNotes = existing.activityNotes) !== null && _existing_activityNotes !== void 0 ? _existing_activityNotes : [],
                    note
                ]
            };
        }
    }
});
const { upsertItem, removeItem, appendActivityNote } = backlogItemsSlice.actions;
// Base selectors
const selectBacklogItemsMap = (state)=>state.backlogItems.items;
const selectBacklogItemById = (state, id)=>state.backlogItems.items[id];
/**
 * Raw `liveVersion` map — returned directly (like `selectBacklogItemsMap`)
 * rather than via `createSelector`: Immer already gives it the same
 * structural-sharing guarantee (unrelated keys keep their prior value
 * across an unrelated item's update), so no extra memoization is needed.
 */ const selectBacklogItemsLiveVersionMap = (state)=>state.backlogItems.liveVersion;
// Memoized selectors: list-shaped reads off backlogItemsSlice must not allocate
// a new array/object on every call, or every consumer (e.g. BacklogItemCard)
// re-renders whenever ANY item in the store changes, not just its own.
const selectAllBacklogItems = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectBacklogItemsMap, (items)=>Object.values(items));
const selectBacklogItemsTotal = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectBacklogItemsMap, (items)=>Object.keys(items).length);
/** Shallow array equality: same length and same element references, in order. */ function shallowArrayEqual(a, b) {
    if (a === b) return true;
    if (a.length !== b.length) return false;
    for(let i = 0; i < a.length; i++){
        if (a[i] !== b[i]) return false;
    }
    return true;
}
/**
 * Selector factory for a status-filtered view. Use one instance per
 * component (e.g. via `useMemo(makeSelectBacklogItemsByStatus, [])`) rather
 * than a single shared selector — a shared selector's single-entry cache
 * would thrash (and stop memoizing) if multiple components filter by
 * different statuses simultaneously.
 *
 * `resultEqualityCheck: shallowArrayEqual` is what actually delivers on the
 * "unrelated item update shouldn't re-render this list" guarantee: without
 * it, the filtered array is a brand-new reference every time ANY item in the
 * store changes (because `selectAllBacklogItems` itself must return a new
 * array whenever the underlying map changes), even if none of the matching
 * items actually differ. With it, the selector still recomputes, but returns
 * the previous array reference when the filtered contents are unchanged.
 */ const makeSelectBacklogItemsByStatus = ()=>createSelector([
        selectAllBacklogItems,
        (_state, status)=>status
    ], (items, status)=>items.filter((item)=>item.status === status), {
        memoizeOptions: {
            resultEqualityCheck: shallowArrayEqual
        }
    });
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (backlogItemsSlice.reducer);


/***/ }),

/***/ 64343:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B: () => (/* binding */ useAbortableEffect)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(2663);
/* __next_internal_client_entry_do_not_use__ useAbortableEffect auto */ 

/**
 * Runs `effect(signal)` on mount and whenever `deps` change, cancelling the
 * previous run first (and on unmount) via useAbortableRequest — the common
 * "declare an AbortController ref, wrap the fetch in useCallback, call it
 * from useEffect" boilerplate collapsed into one call.
 *
 * `effect` still must check `signal.aborted` before every state update
 * after an `await` — this hook only owns cancellation timing, not your
 * component's state (same contract as useAbortableRequest). It intentionally
 * does not manage data/loading/error state itself: hooks in this codebase
 * differ too much in error handling (NotFound stops polling, some errors
 * are non-fatal, some fall back to a cached value) for one generic shape to
 * fit without fighting those differences — see useAbortableRequest's doc
 * comment for the lower-level primitive this builds on.
 *
 * Usage:
 *
 *   useAbortableEffect(async (signal) => {
 *     setLoading(true);
 *     try {
 *       const res = await client.getThing({ id }, { signal });
 *       if (signal.aborted) return;
 *       setData(res.thing);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id]);
 */ function useAbortableEffect(effect, deps) {
    const start = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_1__/* .useAbortableRequest */ .X)();
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const signal = start();
        // effect() is documented to handle its own errors; this catch only
        // guards against a caller that doesn't, so a stray rejection doesn't
        // surface as an unhandled promise rejection.
        void effect(signal).catch(()=>{});
    // effect and start are intentionally excluded: callers list their own
    // fetch dependencies in `deps`, same convention as every hook this
    // replaces (e.g. useSessionVcs.ts's fetchStatus effect).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);
}


/***/ }),

/***/ 68453:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ removeItem),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   B5: () => (/* binding */ addItem),
/* harmony export */   K6: () => (/* binding */ selectReviewQueueLoading),
/* harmony export */   KS: () => (/* binding */ selectReviewQueue),
/* harmony export */   Nh: () => (/* binding */ setError),
/* harmony export */   _W: () => (/* binding */ selectReviewQueueItemsWithLiveStatus),
/* harmony export */   fk: () => (/* binding */ updateItem),
/* harmony export */   r1: () => (/* binding */ setLoading),
/* harmony export */   s6: () => (/* binding */ setReviewQueue),
/* harmony export */   wk: () => (/* binding */ selectReviewQueueLastUpdatedAt),
/* harmony export */   wq: () => (/* binding */ selectReviewQueueError)
/* harmony export */ });
/* unused harmony exports setReviewQueueStats, selectReviewQueueItems, selectReviewQueueStats, selectWaitingItems, selectQueueCountsByWorkingState */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(13009);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/* harmony import */ var _lib_utils_deriveWorkingState__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(81801);



const initialState = {
    reviewQueue: null,
    loading: false,
    error: null,
    lastUpdatedAt: null,
    stats: {
        totalItems: 0,
        byPriority: {},
        byReason: {},
        averageAgeSeconds: "0",
        oldestItemId: "",
        oldestAgeSeconds: "0"
    }
};
const reviewQueueSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSlice */ .Z0)({
    name: "reviewQueue",
    initialState,
    reducers: {
        setReviewQueue (state, action) {
            state.reviewQueue = action.payload;
            if (action.payload) {
                state.stats.totalItems = action.payload.totalItems;
            }
            state.lastUpdatedAt = Date.now();
        },
        setReviewQueueStats (state, action) {
            state.stats = action.payload;
        },
        setLoading (state, action) {
            state.loading = action.payload;
        },
        setError (state, action) {
            state.error = action.payload;
        },
        removeItem (state, action) {
            if (!state.reviewQueue) return;
            const before = state.reviewQueue.items.length;
            state.reviewQueue.items = state.reviewQueue.items.filter((item)=>item.sessionId !== action.payload);
            const removed = before - state.reviewQueue.items.length;
            state.reviewQueue.totalItems = Math.max(0, state.reviewQueue.totalItems - removed);
            state.stats.totalItems = Math.max(0, state.stats.totalItems - removed);
        },
        addItem (state, action) {
            if (!state.reviewQueue) return;
            const exists = state.reviewQueue.items.some((i)=>i.sessionId === action.payload.sessionId);
            if (!exists) {
                state.reviewQueue.items.push(action.payload);
                state.reviewQueue.totalItems += 1;
                state.stats.totalItems += 1;
            }
        },
        updateItem (state, action) {
            if (!state.reviewQueue) return;
            const item = state.reviewQueue.items.find((i)=>i.sessionId === action.payload.sessionId);
            if (item) {
                Object.assign(item, action.payload.updates);
            }
        }
    }
});
const { setReviewQueue, setReviewQueueStats, setLoading, setError, removeItem, addItem, updateItem } = reviewQueueSlice.actions;
// Selectors
const selectReviewQueue = (state)=>state.reviewQueue.reviewQueue;
const selectReviewQueueItems = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    return (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
};
const selectReviewQueueStats = (state)=>state.reviewQueue.stats;
const selectReviewQueueLoading = (state)=>state.reviewQueue.loading;
const selectReviewQueueError = (state)=>state.reviewQueue.error;
const selectReviewQueueLastUpdatedAt = (state)=>state.reviewQueue.lastUpdatedAt;
// selectReviewQueueItemsWithLiveStatus joins review queue items with live session state from
// sessionsSlice. It overrides each item's workingState using the live detectedStatus from
// the WatchSessions stream, so the review queue panel always reflects the current session
// state even when the WatchReviewQueue stream hasn't pushed a fresh snapshot yet.
const selectReviewQueueItemsWithLiveStatus = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_3__/* .createSelector */ .Mz)([
    (state)=>{
        var _state_reviewQueue_reviewQueue;
        var _state_reviewQueue_reviewQueue_items;
        return (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    },
    (state)=>state.sessions.detectedStatusMap
], (items, detectedStatusMap)=>items.map((item)=>{
        const liveStatus = detectedStatusMap[item.sessionId];
        if (!liveStatus) return item;
        const liveWorkingState = (0,_lib_utils_deriveWorkingState__WEBPACK_IMPORTED_MODULE_1__/* .deriveWorkingState */ .m)({
            subStatus: item.subStatus,
            detectedStatus: liveStatus.detectedStatus
        });
        return {
            ...item,
            workingState: liveWorkingState
        };
    }));
// selectWaitingItems returns only items that are NOT actively working,
// so the queue shows sessions that genuinely need user attention.
const selectWaitingItems = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    const items = (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    return items.filter((item)=>{
        const ws = deriveWorkingState(item);
        return ws !== WorkingState.ACTIVE && ws !== WorkingState.PROCESSING;
    });
};
// selectQueueCountsByWorkingState returns per-category counts for the header badge.
const selectQueueCountsByWorkingState = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    const items = (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    let waiting = 0;
    let working = 0;
    let stuck = 0;
    for (const item of items){
        const ws = deriveWorkingState(item);
        if (ws === WorkingState.ACTIVE || ws === WorkingState.PROCESSING) {
            working++;
        } else if (ws === WorkingState.WAITING) {
            stuck++;
        } else {
            waiting++;
        }
    }
    return {
        waiting,
        working,
        stuck
    };
};
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (reviewQueueSlice.reducer);


/***/ }),

/***/ 81801:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   m: () => (/* binding */ deriveWorkingState)
/* harmony export */ });
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/* harmony import */ var _lib_utils_assertNever__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(92418);


/**
 * Derives the effective WorkingState for a session from its SubStatus and DetectedStatus.
 *
 * This is the frontend equivalent of the removed server-side MapDetectedStatusToWorkingState.
 * SubStatus is the primary signal; DetectedStatus is a fallback for cases where SubStatus
 * is UNSPECIFIED (e.g. non-Active sessions or legacy items).
 *
 * Mapping:
 *   SubStatus.PROCESSING, NEEDS_APPROVAL, INPUT_REQUIRED, ERROR, TESTS_FAILING, RATE_LIMITED, WAITING_FOR_AGENT, COMPACTING
 *     → WorkingState.PROCESSING  (actively doing something or blocked on user)
 *   SubStatus.IDLE, READY
 *     → WorkingState.IDLE        (at prompt, ready for next instruction)
 *   SubStatus.SUCCESS
 *     → WorkingState.IDLE        (task finished, prompt available)
 *   SubStatus.UNSPECIFIED → fall through to detectedStatus
 *     DetectedStatus.EXECUTING, WAITING_FOR_AGENT, COMPACTING → WorkingState.ACTIVE
 *     DetectedStatus.PROCESSING                                → WorkingState.PROCESSING
 *     default                                                  → WorkingState.UNSPECIFIED
 */ function deriveWorkingState(session) {
    switch(session.subStatus){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.PROCESSING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.NEEDS_APPROVAL:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.INPUT_REQUIRED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.ERROR:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.TESTS_FAILING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.RATE_LIMITED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.WAITING_FOR_AGENT:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.COMPACTING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.PROCESSING;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.IDLE:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.READY:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.SUCCESS:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.IDLE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.UNSPECIFIED:
            break;
    }
    // detectedStatus-based fallback (used when subStatus is UNSPECIFIED)
    switch(session.detectedStatus){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.EXECUTING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.WAITING_FOR_AGENT:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.COMPACTING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.ACTIVE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.PROCESSING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.NEEDS_APPROVAL:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.INPUT_REQUIRED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.ERROR:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.TESTS_FAILING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.PROCESSING;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.IDLE:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.READY:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.IDLE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.SUCCESS:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNKNOWN:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNSPECIFIED:
        case undefined:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.UNSPECIFIED;
        default:
            return (0,_lib_utils_assertNever__WEBPACK_IMPORTED_MODULE_1__/* .assertNever */ .x)(session.detectedStatus);
    }
}


/***/ }),

/***/ 81921:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   lj: () => (/* binding */ selectRemoteHealthEntry),
/* harmony export */   ng: () => (/* binding */ remoteHealthChanged)
/* harmony export */ });
/* unused harmony export selectRemoteConnectionState */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88149);


const initialState = {
    byName: {}
};
const remotesSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "remotes",
    initialState,
    reducers: {
        remoteHealthChanged (state, action) {
            const { remoteName, state: connState, previousState } = action.payload;
            if (!remoteName) return;
            state.byName[remoteName] = {
                state: connState,
                previousState,
                updatedAt: Date.now()
            };
        }
    }
});
const { remoteHealthChanged } = remotesSlice.actions;
// selectRemoteConnectionState returns a selector for a single remote's
// current connection state, defaulting to UNSPECIFIED (never RemoteConnectionState's
// CONNECTED) when no health-change event has been observed yet -- consumers
// (RemoteConnectionIndicator) must treat UNSPECIFIED as "unknown," not as a
// synonym for disconnected or connected.
const selectRemoteConnectionState = (remoteName)=>(state)=>{
        var _state_remotes_byName_remoteName;
        var _state_remotes_byName_remoteName_state;
        return (_state_remotes_byName_remoteName_state = (_state_remotes_byName_remoteName = state.remotes.byName[remoteName]) === null || _state_remotes_byName_remoteName === void 0 ? void 0 : _state_remotes_byName_remoteName.state) !== null && _state_remotes_byName_remoteName_state !== void 0 ? _state_remotes_byName_remoteName_state : RemoteConnectionState.UNSPECIFIED;
    };
// selectRemoteHealthEntry exposes the full stored entry (state, previousState,
// updatedAt). RemoteConnectionIndicator currently only reads .state from this
// (it tracks its own prior-state via a local ref to decide polite vs. assertive
// announcements, not this entry's previousState) -- exposed as the full entry
// rather than just .state for any future consumer that needs updatedAt/previousState
// directly from the store instead of re-deriving it.
const selectRemoteHealthEntry = (remoteName)=>(state)=>state.remotes.byName[remoteName];
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (remotesSlice.reducer);


/***/ }),

/***/ 92418:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   x: () => (/* binding */ assertNever)
/* harmony export */ });
function assertNever(x) {
    throw new Error("Unhandled case: ".concat(String(x)));
}


/***/ }),

/***/ 93602:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GV: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.GV),
/* harmony export */   M_: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.M_),
/* harmony export */   jL: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.jL)
/* harmony export */ });
/* harmony import */ var _store__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(25337);



/***/ })

}]);
//# sourceMappingURL=2513-9afeb43641233585.js.map