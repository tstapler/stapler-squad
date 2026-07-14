# Changelog

## [1.37.0](https://github.com/tstapler/stapler-squad/compare/v1.36.0...v1.37.0) (2026-07-14)


### Features

* **agy,opencode:** add comprehensive CLI parity for Antigravity and OpenCode ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([38d8a29](https://github.com/tstapler/stapler-squad/commit/38d8a297b67d1fdbb13811a12ce0d7ddb5273222))
* **alias:** add name_prefix field + fix session name oscillation ([9c730be](https://github.com/tstapler/stapler-squad/commit/9c730bec10bbb84e7a8e3ab4a48015b542057602))
* **analytics:** bulk rule creation + page density improvements ([7ae2910](https://github.com/tstapler/stapler-squad/commit/7ae2910e41afd07b0ec1fa3495426a3f8a62d055))
* **analytics:** program detail panel + copilot review fixes ([2774602](https://github.com/tstapler/stapler-squad/commit/2774602a7b4f4cd89baf5d3f8abcabfa21e6a052))
* **analytics:** program detail panel with subcommand drill-down ([29c7af8](https://github.com/tstapler/stapler-squad/commit/29c7af8c25f81b935b048e8a23e27eab06e11a48))
* **analytics:** unify activity tables into single filterable view ([36de4b8](https://github.com/tstapler/stapler-squad/commit/36de4b80f50681cf33e070348eca34f40a457dd5))
* **backlog:** add feedback field to TriggerTriage proto for refine loop ([15d7082](https://github.com/tstapler/stapler-squad/commit/15d70824c0edddf4471bd1f45b2e0a76752f9d57))
* **backlog:** add feedback textarea + Refine action to TriageReviewPanel ([897aa5b](https://github.com/tstapler/stapler-squad/commit/897aa5b3bba4a9f058d2bcfa56bd33d3f1c7e710))
* **backlog:** add hard delete for backlog items ([56483ca](https://github.com/tstapler/stapler-squad/commit/56483ca2ffa65f7b0b60c7a12d40f52df6481875))
* **backlog:** add pr_pending state with GitHub PR push + merge polling ([3714852](https://github.com/tstapler/stapler-squad/commit/37148527b6e6e4a11cbb09ea134195b123ab40a3))
* **backlog:** add review context and View Changes modal to review panel ([92e5d38](https://github.com/tstapler/stapler-squad/commit/92e5d38b1a0b3b8816796f3ab9daa90151cc9d28))
* **backlog:** add UNVERIFIABLE verdict + duplicate session guard + review polling ([67ea766](https://github.com/tstapler/stapler-squad/commit/67ea7660142cfdbfd98963b5bf745ed471ac5325))
* **backlog:** allow backward status transitions to any earlier state ([5b3c4e8](https://github.com/tstapler/stapler-squad/commit/5b3c4e82389e954fa7c4465971e7cebbdc6468eb))
* **backlog:** auto-reopen for rework after FAIL/PARTIAL review; add note to status events ([cf1742b](https://github.com/tstapler/stapler-squad/commit/cf1742b11ec01f41b62d2fafa3da3fa7f2e6966e))
* **backlog:** block review→done when worktree commits exist without a PR ([7111268](https://github.com/tstapler/stapler-squad/commit/7111268a8e691722c5098624286bde1d8eaf833e))
* **backlog:** cap concurrent in-progress items with a WIP limit ([#150](https://github.com/tstapler/stapler-squad/issues/150)) ([91063c4](https://github.com/tstapler/stapler-squad/commit/91063c437871e8d4a4392235f801445477297cc8))
* **backlog:** detect PR merge conflicts and auto-spawn fix sessions ([8413f09](https://github.com/tstapler/stapler-squad/commit/8413f09e1fe2974a6cf1945d80781e08315024a3))
* **backlog:** extend restart to review status + reset to in_progress ([3c9f5dc](https://github.com/tstapler/stapler-squad/commit/3c9f5dc9e045633a1e81ae5f9b99d22509322b8c))
* **backlog:** finish GitHub sync — TriggerSync/GetSyncHistory RPCs, tests, settings UI ([#138](https://github.com/tstapler/stapler-squad/issues/138)) ([9c5927b](https://github.com/tstapler/stapler-squad/commit/9c5927b2551c8fced04723f010aba5424cec97b2))
* **backlog:** fix Changes view + show review criteria for all verdicts + mobile Shift key ([20e70f7](https://github.com/tstapler/stapler-squad/commit/20e70f70a846650a219f2b4146d0e58a2d9e1971))
* **backlog:** fix re-review + add manual review submission ([d164e9e](https://github.com/tstapler/stapler-squad/commit/d164e9e5bc1ce5a2bff7736ffbb778e0b6e421f1))
* **backlog:** gate backlog behind feature flag on all layers ([66b1d83](https://github.com/tstapler/stapler-squad/commit/66b1d831c9d8bcc0df3d9ee654d2fd8822165283))
* **backlog:** gate backlog behind feature flag on all layers ([1186494](https://github.com/tstapler/stapler-squad/commit/1186494baa7d7f9429030f7703ad1e6e4b7e7132))
* **backlog:** git worktree per work session + restart button + review SHA fix ([cea3ae1](https://github.com/tstapler/stapler-squad/commit/cea3ae1d5af69c83d49767fac04261592ec5120a))
* **backlog:** GitHub issue picker — browse repos and issues to import ([4157558](https://github.com/tstapler/stapler-squad/commit/4157558b1309879e570cb5a360288ca24fcaffde))
* **backlog:** ignore injected backlog files in worktrees and repo ([0ee9e14](https://github.com/tstapler/stapler-squad/commit/0ee9e14ac2865261dcb5152fe11ed2331caea049))
* **backlog:** implement CancelTriage RPC and session delete button ([a690c36](https://github.com/tstapler/stapler-squad/commit/a690c366d6704651b913a60dab6b8774303a7f45))
* **backlog:** import backlog items from GitHub issues ([46cf6d4](https://github.com/tstapler/stapler-squad/commit/46cf6d431f8604be9c645b7614278cf73f24bfdf))
* **backlog:** introduce BacklogItemSummary for list-view queries (Epic 5.1) ([10fa01e](https://github.com/tstapler/stapler-squad/commit/10fa01e80094e196db65cbb1a45b10cb08cd90a9))
* **backlog:** PR iteration with CI + review checks, done items visible ([33e54b1](https://github.com/tstapler/stapler-squad/commit/33e54b1195cef1023ba6cfc3d574118d7a8cf068))
* **backlog:** reopen-for-revision spawns new work session with feedback ([0fbf275](https://github.com/tstapler/stapler-squad/commit/0fbf2754c898cbf76168648ff735ba98ab0fe802))
* **backlog:** revision sessions get numbered names and backlog:revision tag ([acef346](https://github.com/tstapler/stapler-squad/commit/acef346f4c59c9819dc7c7bbc5fd2a2ce728dd5b))
* **backlog:** show VCS state, worktree path, and add file/diff navigation ([540bc23](https://github.com/tstapler/stapler-squad/commit/540bc234726f5799e7170516ba497b9b5a189d3e))
* **backlog:** track estimated session cost via existing TokenStore ([658d876](https://github.com/tstapler/stapler-squad/commit/658d876cc17e51b130b71b7c849d1c30f93bdf77))
* **backlog:** wire feedback-driven refine into TriggerTriage ([749cf72](https://github.com/tstapler/stapler-squad/commit/749cf72831c5dd168e5106697dce1aa9d3e86664))
* **backlog:** worktree cleanup, ship skill, branch visibility ([b77665d](https://github.com/tstapler/stapler-squad/commit/b77665dfc73102b5d30df62e27bb1f4ccf0de53f))
* **files:** file browser UX overhaul — mobile, a11y, perf ([169879b](https://github.com/tstapler/stapler-squad/commit/169879b7e631bea2305e2db2e22c637cfc5653ed))
* **files:** wire up the premium LocalFileBrowser component to the files page ([3743b94](https://github.com/tstapler/stapler-squad/commit/3743b945cb6085d160ed5734361b99442a66d9dc))
* GitHub work continuity — persistence, annotation fallback, and type-safe RepoRef ([3be7e09](https://github.com/tstapler/stapler-squad/commit/3be7e0902f57295fb17953d9ff3a84cabcfd3b9f))
* GitHub work continuity — UserPRCache, GitHubUserService, and Unfinished Tab integration ([#141](https://github.com/tstapler/stapler-squad/issues/141)) ([e322a7e](https://github.com/tstapler/stapler-squad/commit/e322a7ebf798ba9e18543fd9d84e396ad065d501))
* **harness:** headless triage test harness + alias kebab-case fix ([7ad3340](https://github.com/tstapler/stapler-squad/commit/7ad33400c273dcc3dc1ec6a210e3cdc4509fa2cf))
* **insights:** link backlog sessions to Insights SessionsTable ([46b66f9](https://github.com/tstapler/stapler-squad/commit/46b66f9799b6c6517deaecdbe3901e214de67a40))
* **nav:** group navigation into 4 sections, restore mobile access ([a2f78a3](https://github.com/tstapler/stapler-squad/commit/a2f78a38698a4a8414d1640a6b0c16795c527418))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([670caef](https://github.com/tstapler/stapler-squad/commit/670caeffd64e6e89aaf13df26c692580aa4effe0))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([250e640](https://github.com/tstapler/stapler-squad/commit/250e64071b9ddb72d44ca85cd6f620988f6bfb33))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([bc794b7](https://github.com/tstapler/stapler-squad/commit/bc794b7fc4e88b39dbc86ece70a5c086e48d47b6))
* **onboarding:** offer to install Claude Code hooks during onboarding ([#138](https://github.com/tstapler/stapler-squad/issues/138)) ([41e0206](https://github.com/tstapler/stapler-squad/commit/41e0206f54acbf58203959fbc697e173cb7471fb))
* **perf:** add singleflight + hasUncommitted TTL cache to GoGitVCSReader ([6fc0fb2](https://github.com/tstapler/stapler-squad/commit/6fc0fb20f187116698196946f82d707e3dc21f7a))
* **perf:** invalidate IsDirty cache on session Pause and Resume ([4cee38d](https://github.com/tstapler/stapler-squad/commit/4cee38dc0ffd2f164ce19dad01e4fab7fb543955))
* **pr-status:** show PR badge in row mode and use go-git for branch detection ([664fbfb](https://github.com/tstapler/stapler-squad/commit/664fbfb118d036a7ef52e0519a05d9e8368f22ca))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([837ba8c](https://github.com/tstapler/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **rules:** auto-suggest rule name from criteria inputs ([#140](https://github.com/tstapler/stapler-squad/issues/140)) ([d072abc](https://github.com/tstapler/stapler-squad/commit/d072abc37fcb228dd052cd38cb8c324eb8c3bdcf))
* **services:** extract CheckpointSvc, TerminalSvc, FeatureFlagSvc; delegate ListBranches+workflow (CDD Epic 1) ([8936b3a](https://github.com/tstapler/stapler-squad/commit/8936b3afc3ad039de23ee952e80979efdd45ca5a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([3571d03](https://github.com/tstapler/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** actor goroutine + mailbox + sendSync + goleak verification (IAC Epic 3) ([8adc0c9](https://github.com/tstapler/stapler-squad/commit/8adc0c9be7c2699fc506b365ba4f0bdd45e18790))
* **session:** bidirectional session history transfer between Claude and Antigravity ([#130](https://github.com/tstapler/stapler-squad/issues/130)) ([08d3f87](https://github.com/tstapler/stapler-squad/commit/08d3f87b06a954acff5ab0a567defe97db407bed))
* **session:** confirm dialog + re-sync guard for the program-change picker ([f781d7c](https://github.com/tstapler/stapler-squad/commit/f781d7cd3ac3640a3984282be3092294cff77880))
* **session:** IAC Epic 4 — actor-path state-machine core migration ([7bcc479](https://github.com/tstapler/stapler-squad/commit/7bcc479642a682671acd26d80cb618779614e418))
* **session:** InstanceSnapshot + atomic.Pointer read path, snapshot publish in all mutators (IAC Epic 1) ([e8cfafc](https://github.com/tstapler/stapler-squad/commit/e8cfafc44a56c217b4bfdb03a69b418ea8124ecd))
* **session:** migrate all unguarded readers to snapshot.Load() (IAC Epic 2) ([f491483](https://github.com/tstapler/stapler-squad/commit/f491483a76b7b5ff1fef2d6dc2131ff882ebe5ba))
* **session:** pending-program-change badge on the session card ([442380e](https://github.com/tstapler/stapler-squad/commit/442380e9b904c3bf6b9e971666a5e6f51ac34cb8))
* **session:** Registry + LiveInstance type-split lifecycle layer (IAC Epic 2.5) ([4232806](https://github.com/tstapler/stapler-squad/commit/42328063146596b88000b022ce0022f9e6341653))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([b695764](https://github.com/tstapler/stapler-squad/commit/b695764e9a5d12827da7db0815d81db262288dd9))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([f5f7f36](https://github.com/tstapler/stapler-squad/commit/f5f7f3618271b62783ca906274a5d4cac8eae13c))
* support Antigravity CLI hooks.json format in ssq-hooks ([55df54d](https://github.com/tstapler/stapler-squad/commit/55df54d4f60be8cddf6082c8ed64621f60db26de))
* **tmux:** add cross-process concurrency gate for tmux subprocess execution ([00988ac](https://github.com/tstapler/stapler-squad/commit/00988acc82fd3b344dcb3a1570cc037c56b7f3f6))


### Bug Fixes

* address copilot review comments on analytics drill-down ([077f53f](https://github.com/tstapler/stapler-squad/commit/077f53f6e075ef467f61022a3be28321f56cc387))
* **alias:** default session type + name oscillation ([4d33c7d](https://github.com/tstapler/stapler-squad/commit/4d33c7d50c4cd66ed4cbbd585d9556329259f050))
* **analytics:** escape analytics session_id mismatch and dead mangle detection ([#149](https://github.com/tstapler/stapler-squad/issues/149)) ([a121de8](https://github.com/tstapler/stapler-squad/commit/a121de83717185a5a42183288ffde1f8e0a298cf))
* **approval:** inject PermissionRequest hook on CreateSession and RestartSession ([8238a64](https://github.com/tstapler/stapler-squad/commit/8238a64c57033833615cfa7dc83c26f2db57bd0b))
* **arch:** architecture review fixes — ctx ordering, lint, and recovered stash deletions ([d63388e](https://github.com/tstapler/stapler-squad/commit/d63388e410604a6866350436fefc45bfbbfc6d25))
* autonomous sessions rejected with "path is required" via omnibar ([#157](https://github.com/tstapler/stapler-squad/issues/157)) ([5ab6c4a](https://github.com/tstapler/stapler-squad/commit/5ab6c4a08a03dbb6b28e5ae98975a72b543878d6))
* **autonomous:** send raw \r instead of \r\n for turn injection ([b758451](https://github.com/tstapler/stapler-squad/commit/b758451e6fc790dfa3dd4c3e757085a09336c950))
* backlog/triage sessions die on launch (shell injection + flag-parsing crash) ([#150](https://github.com/tstapler/stapler-squad/issues/150)) ([8016921](https://github.com/tstapler/stapler-squad/commit/8016921fa302c01ed9c16c5f568e410b9081b2b7))
* **backlog:** add SessionVcsProvider to ReviewChangesModal and fall back to DB for GetSessionDiff ([3da4a42](https://github.com/tstapler/stapler-squad/commit/3da4a42c25d21ae16b27497ca3127f1c08e812aa))
* **backlog:** address architecture-review follow-ups from post-merge audit ([#142](https://github.com/tstapler/stapler-squad/issues/142)) ([7791548](https://github.com/tstapler/stapler-squad/commit/7791548c1f3b633033975cdc4b2d6b44f0a7fc17))
* **backlog:** backfill pr_number when set to 0 despite a real pr_url ([4e411f5](https://github.com/tstapler/stapler-squad/commit/4e411f5b29bd137542d39467fa0c7d3850a68e6e))
* **backlog:** capture base SHA at session spawn so review sees full diff ([b6ff13a](https://github.com/tstapler/stapler-squad/commit/b6ff13aadf703e51e18901f8deab2661ad6e41a3))
* **backlog:** capture base SHA in AttachSessionToItem + fix WorkingDir race ([30c2590](https://github.com/tstapler/stapler-squad/commit/30c2590990ab35d2bd974be11856e0d102a5ae52))
* **backlog:** categorize backlog-spawned sessions as "Backlog" ([#149](https://github.com/tstapler/stapler-squad/issues/149)) ([f65c4a1](https://github.com/tstapler/stapler-squad/commit/f65c4a1796d6a7481989d2554f66cf5b37d630a6))
* **backlog:** enable Reopen for Revision in pending state, add files tab UX improvements ([859b71a](https://github.com/tstapler/stapler-squad/commit/859b71aa58e1ef9d5ee1cdd91b41788e93395eb9))
* **backlog:** fix approve flow and synthetic session display ([de58992](https://github.com/tstapler/stapler-squad/commit/de58992337c9caa60ab12d73d8382efa14fa6e44))
* **backlog:** fix review gate receiving empty diffs + restore sessionCreator nil guard ([9436a9a](https://github.com/tstapler/stapler-squad/commit/9436a9a3aefcec50cd094a31a14ccd5f9ef4096d))
* **backlog:** fix triage sessions tombstoned immediately on GetBacklogItem ([dc131c2](https://github.com/tstapler/stapler-squad/commit/dc131c2d45f27228e7d0ee77a2e10f3fa74cb760))
* **backlog:** fix triage spinner, delete, nav, stderr capture, and artifact dir ([aebc872](https://github.com/tstapler/stapler-squad/commit/aebc87204568de026e62dd70644b3816d6a25efd))
* **backlog:** GitHub URL repo-path support, first-visit tour, and two related bugs ([#152](https://github.com/tstapler/stapler-squad/issues/152)) ([6ef8164](https://github.com/tstapler/stapler-squad/commit/6ef8164b215f0c08c4a497b00fd33da516e9a288))
* **backlog:** harden claude binary lookup against a stale Linux PATH ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([6839082](https://github.com/tstapler/stapler-squad/commit/68390826f0a22d380c04a1163c65017a113e8583))
* **backlog:** harden the review-&gt;PR-&gt;merge reconciler and autonomous session UX ([bb5fa80](https://github.com/tstapler/stapler-squad/commit/bb5fa80ebc1ea520708606bc498da19b5e96cb03))
* **backlog:** harden triage JSON parser against stray braces in preamble ([#134](https://github.com/tstapler/stapler-squad/issues/134)) ([fd56d6a](https://github.com/tstapler/stapler-squad/commit/fd56d6ae1e5fa73a86c42b2ef245c0ca7d10fba3))
* **backlog:** harden triage parser and add repoPath UI gate ([06f9857](https://github.com/tstapler/stapler-squad/commit/06f98573b53af59afab38dfb0684edcc59210531))
* **backlog:** include repo name in spawned session title ([880af9e](https://github.com/tstapler/stapler-squad/commit/880af9ef378633283ce29f30b76e1c12d35a90ac))
* **backlog:** inject MCP config when spawning backlog work sessions ([3ccfc42](https://github.com/tstapler/stapler-squad/commit/3ccfc42a57f7852ed06ce8ee7bc5527389932a2a))
* **backlog:** make autonomous mode properly integrated with backlog store ([a84e89b](https://github.com/tstapler/stapler-squad/commit/a84e89bb25053510c8d718289434f21d71cbcc26))
* **backlog:** review gate produces empty/wrong diffs for fast work sessions ([39f3f7b](https://github.com/tstapler/stapler-squad/commit/39f3f7b94f803e544af0ddfa2bdef273b76adcf1))
* **backlog:** run autonomously + lifecycle safety nets ([19f35ac](https://github.com/tstapler/stapler-squad/commit/19f35ac9e8864f09b884c1c43c3c7ec165b15fbe))
* **backlog:** skip gitManager.Setup() for ExistingWorktree sessions ([06a6b46](https://github.com/tstapler/stapler-squad/commit/06a6b4677aa14367f079e25998a0efd1b47af8de))
* **backlog:** store AC notes in separate field, use nudge grace period ([3705e48](https://github.com/tstapler/stapler-squad/commit/3705e4885a07ab407f3d71bdbbb409afe4a7f14d))
* **backlog:** surface work-session verification evidence to the review gate ([#152](https://github.com/tstapler/stapler-squad/issues/152)) ([70e33b0](https://github.com/tstapler/stapler-squad/commit/70e33b0d077506fef2e0942c9046edf258b9558a))
* **backlog:** triage now writes ACs; review includes plan; AC merge semantics ([d1103aa](https://github.com/tstapler/stapler-squad/commit/d1103aa1c20a44e27cd7d5b5a05edd2041e97de0))
* **backlog:** TriggerTriage's cleanupCtx expired before it was ever used ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([6552ec9](https://github.com/tstapler/stapler-squad/commit/6552ec9039b1a9f364e874e552ecb9cea7f04530))
* **backlog:** uncommitted-changes guard + TriggerTriage race fix ([15f1f9e](https://github.com/tstapler/stapler-squad/commit/15f1f9e36a581fcc8941129c7e840c58f59d0ecb))
* **backlog:** use auto permission mode, short session names, autonomous tag ([c770fcd](https://github.com/tstapler/stapler-squad/commit/c770fcd5bf914b105f232de4be723780944590cc))
* **backlog:** use earliest work session SHA in TriggerReReview ([447a056](https://github.com/tstapler/stapler-squad/commit/447a056b2282e1f758adf65821e65a33c1b4d904))
* **backlog:** use worktree path and base SHA for all review diffs ([82cf2ea](https://github.com/tstapler/stapler-squad/commit/82cf2ea0c7a02fbc05fdafc838745f1f4f250601))
* **backlog:** wire review sessions + recover stuck review items ([d7e05c7](https://github.com/tstapler/stapler-squad/commit/d7e05c7e012a6a60ae673e27d3a19a7542797d7d))
* **build:** decouple lint from binary build; fix proto duplicate + GitHub RPC stubs ([cb20700](https://github.com/tstapler/stapler-squad/commit/cb20700b95b8ffa28eb60b55e388e370e08e00ea))
* **ci:** resolve lint/build failures on main ([e656a02](https://github.com/tstapler/stapler-squad/commit/e656a02cfcc7df04db6b43800950a358daf7fccb))
* **ci:** serve the static export in Lighthouse CI instead of next start ([8470143](https://github.com/tstapler/stapler-squad/commit/847014300b28cfe2ebd978efac761d8a5f3601eb))
* **codesign:** correct otool byte-order in verify-codesign plist decode ([ca60c0a](https://github.com/tstapler/stapler-squad/commit/ca60c0ae893ba542a62cc46c5789af38fb57da4c))
* **css:** enable scroll on unfinished tab container ([fa5f37e](https://github.com/tstapler/stapler-squad/commit/fa5f37e25607ca094858655a57cbc0ca9202f368))
* **detection:** detect dynamic workflows + expand turn-marker to ✦ ([2fad2c0](https://github.com/tstapler/stapler-squad/commit/2fad2c0eacc453b5a729b05d185774007622095f))
* **files:** inject &lt;base&gt; tag into HTML served by /api/files/raw ([2bfbf17](https://github.com/tstapler/stapler-squad/commit/2bfbf1705731270c8b9141dc3ee3491b7c0031fc))
* **files:** rewrite relative HTML asset URLs instead of injecting &lt;base&gt; tag ([49e391d](https://github.com/tstapler/stapler-squad/commit/49e391d4954eaa7b1c8ab212d2ba8ce35c42e0ff))
* **github:** centralize rate-limit handling in transport; read all rate-limit headers ([7fd1d77](https://github.com/tstapler/stapler-squad/commit/7fd1d7703d174943e5eb474bdac97982b883ab63))
* **github:** prevent data race in UserPRCache.Start() via sync.Once ([9ec03fb](https://github.com/tstapler/stapler-squad/commit/9ec03fb5ce53102cc7243e8dfd946063f175e2bb))
* **github:** remove duplicate maxRetryAfterSleep declaration ([c9b21aa](https://github.com/tstapler/stapler-squad/commit/c9b21aa759cab7d56dbf0c5bc6708264131488c5))
* **github:** use conditional PR discovery in PRStatusPoller + fix 403 error messages in repos.go ([c65ffbc](https://github.com/tstapler/stapler-squad/commit/c65ffbcafcc3183d2ec4ac45c196a85266671487))
* **git:** shell out to git CLI for HEAD SHA instead of go-git ([#151](https://github.com/tstapler/stapler-squad/issues/151)) ([70fe264](https://github.com/tstapler/stapler-squad/commit/70fe264d73257626762d735d4b9d25b27baf8d83))
* **git:** use fmt.Fprintf instead of WriteString(fmt.Sprintf(...)) ([7909e2f](https://github.com/tstapler/stapler-squad/commit/7909e2fbef3960a1bf5f83f2814a6859d8825672))
* **headless:** use Setsid instead of Noctty for headless runner subprocess ([095e09e](https://github.com/tstapler/stapler-squad/commit/095e09e33e2d01b9164cc5e0da707ed9fa83f35f))
* **install:** skip FDA prompt for non-admin users with cert-signed binary ([c24d088](https://github.com/tstapler/stapler-squad/commit/c24d088cd1bdb72225f0ac81cb16846b7d637c2a))
* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([3407f0c](https://github.com/tstapler/stapler-squad/commit/3407f0c161e3132362be9f175118be02a692480a))
* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([43110eb](https://github.com/tstapler/stapler-squad/commit/43110ebdf64d99b3ebe3bd6f1fc5adefeee190b9))
* **lint:** analytics-exempt line order; chore(ci): make demo GIFs manual-only ([1255f16](https://github.com/tstapler/stapler-squad/commit/1255f168a5ecbebc7c1d3ad88c58fdd1e0401711))
* **lint:** move analytics-exempt to line 1 in backlog-sources page ([51ced91](https://github.com/tstapler/stapler-squad/commit/51ced915db251b07b68f575fe3e96bb71b042dca))
* **lint:** resolve 4 violations surfaced after golangci-lint go1.26 rebuild ([9c69d79](https://github.com/tstapler/stapler-squad/commit/9c69d797ab0a4fec0461a9e9223af32fdc34eaba))
* **lint:** resolve blocking ESLint errors surfaced by push-to-main full scan ([64cc150](https://github.com/tstapler/stapler-squad/commit/64cc150fd84908ee70d576adb40cd1e3ebbf85fe))
* **lint:** return empty map instead of nil in GetAllInstanceArtifacts ([c8b9780](https://github.com/tstapler/stapler-squad/commit/c8b97803aa46a02fa3b801ee073f1f45a2232a0e))
* **lint:** suppress norawexec on lookPathOnlyExecutor stub ([d054495](https://github.com/tstapler/stapler-squad/commit/d054495cb49d5a4151475df6fa0fe2e8cb99edc2))
* **lint:** use correct nolint directives for lookPathOnlyExecutor stub ([8987673](https://github.com/tstapler/stapler-squad/commit/898767374dad24c5c6ab8e30aecf4ec5c96c5c29))
* **lint:** widen forbidigo + depguard exclusions to cover P1 split files ([1f3a64e](https://github.com/tstapler/stapler-squad/commit/1f3a64e3590e552387924acac9f2813a5e899ff0))
* **mcp:** transition item to review status in request_review tool ([003e078](https://github.com/tstapler/stapler-squad/commit/003e07857736162b4461cdd8a83e27834e368138))
* **mcp:** use stateless session mode to survive server restarts ([9f462de](https://github.com/tstapler/stapler-squad/commit/9f462dea91dddb6dd95e12b3f2f9c152198036a0))
* **mcp:** write project MCP config to .mcp.json instead of settings.local.json ([7d47dbb](https://github.com/tstapler/stapler-squad/commit/7d47dbbdd1ad6d932ebbb1b6dd68db48722c26b6))
* **mobile:** prevent horizontal scroll on notifications and other pages ([759a100](https://github.com/tstapler/stapler-squad/commit/759a1006c2f4ad90b304793293dc89d46a693ac1))
* **nav:** add Feature Flags to navigation menu ([3fa665c](https://github.com/tstapler/stapler-squad/commit/3fa665cf20b250f51d09bb880fdcd9f1a7164c94))
* **omnibar,tmux:** kebab-case session names and fix tmux name mismatch ([#162](https://github.com/tstapler/stapler-squad/issues/162)) ([0c805fb](https://github.com/tstapler/stapler-squad/commit/0c805fb41c52f5d18cfc76fa829e823316959c52))
* **pane:** restore session peek modal integration in pane picker ([6aef840](https://github.com/tstapler/stapler-squad/commit/6aef84015c5dba709b17f812b8b1810abb601f5e))
* **perf:** downgrade snapshot-cache hot-path logging to Debug ([119782a](https://github.com/tstapler/stapler-squad/commit/119782a31758e4bbd9c5088d66539957f276f81b))
* **perf:** release entry.mu before OS stat walk in HasUncommitted; typed nil returns in Do bodies ([71f52b3](https://github.com/tstapler/stapler-squad/commit/71f52b39bc907b69b7c6103db641ed59c6e92db0))
* **perf:** rename misleading panic test, add scope comment, move InvalidateDirtyCache post-transition ([799a352](https://github.com/tstapler/stapler-squad/commit/799a3521a5a040772cf9aa23a071b4a84361eaf0))
* **poller:** add rate-limit backoff, no-PR backoff, and ETags to WorktreePRPoller ([3f4c5ca](https://github.com/tstapler/stapler-squad/commit/3f4c5caf11a25a6f717c0f15d30663b5ee92abea))
* **process:** reap tmux control-mode children on ungraceful parent death ([2f93900](https://github.com/tstapler/stapler-squad/commit/2f93900cc6cb001bd5a5ac0a45527e52350e2207))
* publish session update event on controller status change ([9241d3e](https://github.com/tstapler/stapler-squad/commit/9241d3e0bcb89c0f65971f64435f4afe4fab1064))
* **registry:** add alias RPCs to scanner methodToID map ([a1ac791](https://github.com/tstapler/stapler-squad/commit/a1ac791b11dfa3d38dfa81ee56106221286b9987))
* repair broken release pipeline and build-from-source path ([#147](https://github.com/tstapler/stapler-squad/issues/147)) ([1fc2ff6](https://github.com/tstapler/stapler-squad/commit/1fc2ff6c90fe88af32f14fa600bdfc03a63fcf54))
* repair main — duplicate actor declarations + orphaned config-file-rules stubs ([#140](https://github.com/tstapler/stapler-squad/issues/140)) ([9fa5059](https://github.com/tstapler/stapler-squad/commit/9fa5059d1da8090680499d35113a9cb912bcf049))
* review queue auto-advance respects preference after approve/deny ([c63886a](https://github.com/tstapler/stapler-squad/commit/c63886aa90a2354e41140a51280ad86eadfc06df))
* **review-gate:** include staged/unstaged changes in GetGitDiff ([c0e57ca](https://github.com/tstapler/stapler-squad/commit/c0e57ca1411f7e814126a896b61aa147031517f3))
* **review-gate:** two PASS/UNVERIFIABLE transition bugs ([98c8e8a](https://github.com/tstapler/stapler-squad/commit/98c8e8a9b4f43042d2e6b887c0425dd2247feebe))
* **review-queue:** resolve UUID→Title before Remove so approved/deleted sessions leave the queue ([ea94659](https://github.com/tstapler/stapler-squad/commit/ea94659ff329737bc7a15fab2bfb9c927a2ed943))
* **review-queue:** show INPUT_REQUIRED items + UX improvements ([757666a](https://github.com/tstapler/stapler-squad/commit/757666aff79b654bcb8a52468ce7a26778ebeb11))
* **review-queue:** suppress auto-advance on session status transitions ([c3fa5e3](https://github.com/tstapler/stapler-squad/commit/c3fa5e3f523d070ccbada952a157a36b67206335))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/tstapler/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **server:** bound shutdown-hook execution to prevent SIGKILL on restart ([9631fd1](https://github.com/tstapler/stapler-squad/commit/9631fd1b8b09267d893324120146bc0247411506))
* **service:** fall back to launchctl load when bootstrap fails on macOS ([46bd0ea](https://github.com/tstapler/stapler-squad/commit/46bd0ea9f5d92153f7bb279f56b2cbc0fcfdca56))
* **session-driver:** add live output check before prompt injection ([aa9834c](https://github.com/tstapler/stapler-squad/commit/aa9834cdc11b87460eb2d706946cc1c4e07eac2f))
* **session:** defer session Start() so the HTTP server binds immediately ([cc0903e](https://github.com/tstapler/stapler-squad/commit/cc0903e554cc14ceb4d5e79c1c765de6d5e70335))
* **session:** hot-attach to already-running tmux session on restore ([7d42363](https://github.com/tstapler/stapler-squad/commit/7d42363cac0e39c243b2bb3572b2f0b57c3cff64))
* **session:** program switching now saves correctly for all cases ([914138e](https://github.com/tstapler/stapler-squad/commit/914138ec162e494f9d90818182c93d3bb0a2ae53))
* **session:** release stateMutex before calling Start() in SwitchWorkspace ([f6f89d8](https://github.com/tstapler/stapler-squad/commit/f6f89d84868fc89e9242f6d3c68541548295dfbc))
* **session:** replace nil,nil return with ErrInstanceDataNotFound sentinel (nilnil lint) ([e2248ba](https://github.com/tstapler/stapler-squad/commit/e2248bac6bf0e8d8efc9a2ad7269dfad2a268525))
* **session:** respawn panes killed under remain-on-exit ([3d7c075](https://github.com/tstapler/stapler-squad/commit/3d7c075677be45b6f7f30faa84954a13817535fc))
* **session:** serialize SwitchProgram to prevent race on concurrent switches ([16bdf0f](https://github.com/tstapler/stapler-squad/commit/16bdf0f5a1447b1ed228a5297e130b80d95a14b0))
* **sessions:** prevent Claude process orphaning after server restart ([da222c2](https://github.com/tstapler/stapler-squad/commit/da222c259e1303a9b084489d550138fbb4539074))
* **session:** stop discarding the program-change save promise, surface save errors inline ([a478983](https://github.com/tstapler/stapler-squad/commit/a47898328421f5080ba0a8e79413269ee4d5576b))
* **session:** stop repeated "1" keystroke on directory-approval prompts ([#146](https://github.com/tstapler/stapler-squad/issues/146)) ([7948dae](https://github.com/tstapler/stapler-squad/commit/7948daebae68406b15d67bb9669e1ac0fc7eef6e))
* skip shell sourcing in test mode to prevent service test timeout ([49eb0ba](https://github.com/tstapler/stapler-squad/commit/49eb0ba5dab6438f9796f36fa6ff8da55b10875a))
* **terminal:** correctly scan OSC/DCS escape sequences to stop render artifacts ([#156](https://github.com/tstapler/stapler-squad/issues/156)) ([2151b4b](https://github.com/tstapler/stapler-squad/commit/2151b4b2186627c64c1be1617e4e096918d5b491))
* **terminal:** repair escape code pipeline for new Claude Code renderer ([#139](https://github.com/tstapler/stapler-squad/issues/139)) ([dc71b82](https://github.com/tstapler/stapler-squad/commit/dc71b828a6d6a61883cd12b7c666e836eda97c90))
* **test:** prevent tests from spawning a real, unbounded claude agent ([cd124e7](https://github.com/tstapler/stapler-squad/commit/cd124e7bfde6b863cd382b60a82155574c3c40f1))
* **tests:** recognize shared isolated tmux socket in leaked-server reaper ([f3b7ce4](https://github.com/tstapler/stapler-squad/commit/f3b7ce43018f825fb6f13763633de10114ba2201))
* **tmux:** always include session UUID in MCP HTTP config header ([63e21a7](https://github.com/tstapler/stapler-squad/commit/63e21a7743a7b9470942c370d17d872bdd5a907f))
* **tmux:** coalesce concurrent DoesSessionExistNoCache calls ([a9616bf](https://github.com/tstapler/stapler-squad/commit/a9616bfea47eea2268db1b27591e45a2fe9f2a0b))
* **tmux:** flush stale exists-cache in RestoreWithWorkDir and add --tmux-keep-server to plist ([12b992a](https://github.com/tstapler/stapler-squad/commit/12b992a32ca5e9997f6b042e719bc49ef99eec91))
* **triage:** silent storage error, hung session timeout, and Claude detection false positives ([cc82510](https://github.com/tstapler/stapler-squad/commit/cc8251090c0d6987e8f439fb1ba7b8cbce5d41e7))
* **ui:** prevent N×5 dispatch storm freezing home page on load ([6d0859d](https://github.com/tstapler/stapler-squad/commit/6d0859d26265c6f4c11cf12f9e1a3371f3635090))
* **unfinished:** stack GitHub auth banner vertically so Connect button is always visible ([750d252](https://github.com/tstapler/stapler-squad/commit/750d252092abf455d7c20ad8ad143486e28674d2))
* **ux:** address 8 UX review findings from 2026-06-30 ([4780b94](https://github.com/tstapler/stapler-squad/commit/4780b94df04df4fc3334bd0d238a269601a95574))
* **vcs:** periodically clear go-git repo cache to bound memory usage ([6a3e5dd](https://github.com/tstapler/stapler-squad/commit/6a3e5dd13811bd4416468ba67b0f656c385ac0f8))
* web-build target doesn't generate proto bindings on a clean clone ([#155](https://github.com/tstapler/stapler-squad/issues/155)) ([1bb135e](https://github.com/tstapler/stapler-squad/commit/1bb135e8d93456c70db12ceb77b9f335369925f9))


### Performance Improvements

* **mcp:** make stapler-squad --mcp a thin proxy of the running server ([6d6d1aa](https://github.com/tstapler/stapler-squad/commit/6d6d1aa8aa5747a9e6fd648fe121e0a3a1bdfff0))
* **tmux:** add semaphore to cap concurrent capture-pane subprocesses ([fa54896](https://github.com/tstapler/stapler-squad/commit/fa548964a70cc757ac46a5d49b810ca389b53f90))
* **unfinished:** cut go-git diff scanner allocations and bound worst-case cost ([#153](https://github.com/tstapler/stapler-squad/issues/153)) ([a45fa20](https://github.com/tstapler/stapler-squad/commit/a45fa20f6ed92afae1f928c381edeeaf71221c56))
* **vcs:** cache reachableSet results and batch-read blobs under single lock ([125f4ef](https://github.com/tstapler/stapler-squad/commit/125f4ef7e3f32f0a1f05c213cd394e4ab2e5e3bd))


### Reverts

* remove incorrect InjectMCPConfig from SpawnSessionFromItem ([0595394](https://github.com/tstapler/stapler-squad/commit/05953949b118b91c27ea892126b086283fb6e8fe))
* remove incorrect per-session hook injection in session_service ([cfdf4ff](https://github.com/tstapler/stapler-squad/commit/cfdf4ffb2d2ded1fb4604742da83f20ab44378c7))

## [1.36.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.35.0...v1.36.0) (2026-07-10)


### Features

* **analysis:** add gVisor checklocks + annotate critical mutex-protected fields ([bea8cde](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bea8cde8d627a3aa2bbb6fc2e08114eff52e3819))
* **tmux:** add Socket value type and tmuxsocketscope lint pass; fix remaining gaps ([5b61c3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5b61c3e7fb9460ad1167bf3c89ebf0fc0fbb2cc3))


### Bug Fixes

* **build:** download tmux configure from release tarball, add ensure-tmux-configure target ([5612c89](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5612c892b91c528542fc842e86e45a3b94b3c196))
* **server:** stop integration tests from killing production tmux sessions ([bf9e78e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bf9e78e96bc584f45f0c098d6c681d709ba81a02))
* **session:** resolve tmux multi-socket reconcile bug and hibernation resurrection bug ([666d8ad](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/666d8ad39d1f3b13c474f5bb084595107f353cf4))
* **tmux:** close remaining gaps in per-process tmux socket isolation ([8885672](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/88856728d2d0f7c5f2b9f27a3e5b9440d90876ec))


### Performance Improvements

* **adapter:** compute GetStatus once per InstanceToProto instead of 3x ([4836f7a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4836f7abfb933f2df2a3babf6a4eebef63ada8a3))
* **analytics:** drop hex.EncodeToString per parsed escape sequence ([81fc691](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/81fc691c9d05fd4bdb67447bcb13f70c15565177))
* **analytics:** eliminate string allocs in charset and SGR description paths ([7d463f6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7d463f6423d3b3eba5037a2a5b7cfe0d8295ee9b))
* **analytics:** replace fmt.Sprintf string keys with zero-alloc struct keys ([b7f7807](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b7f7807e5c8fb7707d7b59afdf73103d9a1ee0f5))
* **analytics:** return ParsedEscapeCode by value to eliminate per-sequence heap alloc ([f350b4c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f350b4c5dea98065f79cdee544917bda77ea32d2))
* **analytics:** skip all extraction when escape writer is Noop (no DB configured) ([163dbe2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/163dbe21642c4d89f07995c526871be5cafd8fbd))
* **analytics:** skip escape extraction in Parse when captureLevel is off ([7ebce9f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7ebce9fa439447301d50108331626c565ba30510))
* **analytics:** skip mutex in EscapeCodeStore.Record when store is disabled ([e88d819](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e88d819ee3404d0a54cf97e1aeb90f3b27170bdd))
* batch tag updates, eliminate bytes.Buffer in decode, strings.Cut on hot path ([3b121f9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3b121f9114e4be89c8552c9a8e8d54f08a5e53a5))
* **buffer:** make TotalBytesWritten lock-free via atomic.Int64 ([a5e8a0d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a5e8a0d09d4941a8dd8e78a1a0a382a80144d304))
* cache capture-pane/PanePID, lazy-load SessionDetailView, memoize SessionRow ([8d816ea](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8d816ea806bb15d9eed2f8b700496158e2154abf))
* cache GetRemoteURL, GH keychain token; increase IsDirtyCacheTTL to 30s ([74e343b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/74e343b8c0feedf7ef84b4c0be664b153d0bc5ef))
* cacheMu Mutex→RWMutex in pollerContentProvider for concurrent cache reads ([bb3cb6b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bb3cb6b7b72b4e0459e011b26cd47e94ebc13196))
* **cache:** replace 4-map+RWMutex with xsync.MapOf in pollerContentProvider ([13f3230](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/13f32303f492d574a0ebab9fb6ebc5549f28c1a7))
* **cache:** replace Locked[cacheState] with atomic.Pointer in hot poll path ([0cbcab4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0cbcab45750b918294a6c9371571e7fd425e4371))
* **cache:** skip 4KB alloc on status cache hit via GetRecentHash ([79525c6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/79525c60a8cf0a717c6ad8307140a69a435ff1e7))
* **controller:** add started atomic.Bool + filterTmuxMetadata fast path ([7259e6b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7259e6ba987c06480959fd4fa6aa46849b6b6ca7))
* **controller:** fix GetRecentOutput(n) to read only n bytes, not full 10MB buffer ([f1c6580](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f1c6580a416b523defa3d304d26a2f50b5bc0f2e))
* **detect:** eliminate string(raw) alloc in hasScreenOverwrite ([5012711](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/50127111c88ef61619e219beba6c21b444d83f5f))
* **detection:** remove PatternSet mutex — struct is immutable after construction ([54dc79c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/54dc79c14a30b1395c7aecd242c2abbcc736a54b))
* **detection:** replace patternSet RWMutex with atomic.Pointer ([92e5151](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/92e51515ee1fb8f814408948dfb3ca4e81361159))
* **detect:** narrow lock scope in DetectStateFromContent/DetectState ([66633e0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66633e0cd45089cca222107e1ff9833ac3b534cd))
* **detect:** scan lastNLines backward to avoid splitting discarded prefix ([e470e15](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e470e15f475ccb6eb1b778433189a85f4fe60ef0))
* **detect:** skip []byte round-trip in detectFromLines via string variant ([a4d24a9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a4d24a9bf58b3665e342606a89f1a5823cdadc66))
* **detect:** use IndexByte instead of ContainsRune for ASCII '\r' scan ([ea572e6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ea572e6ecacebc90a996883fe78e29458584e4fa))
* **dirty:** replace sync.RWMutex+2fields with atomic.Value on IsDirty fast path ([2ce0f7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2ce0f7e135ecf0df57fffde4db0587a94cf7c947))
* GetQueuedCommandsCount avoids slice alloc, drop Sprintf on hot status path ([c04db64](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c04db6414a23b7330d3e55f4c22722287472970d))
* **hash:** avoid []byte copy in ComputePromptSignature via unsafe.StringData ([c286c3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c286c3e43c78155c4792ca2cc6ec2857d2f072db))
* **hash:** replace FNV+alloc with murmur3 for status cache keys ([0ea1f63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0ea1f6305f7e67e9a4e507202884e70f3ee8104a))
* HasMeaningfulContent strings.Cut iteration, zero alloc vs strings.Split ([50c0256](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/50c0256641a94d9ef600d70c95d14a973c6b68ae))
* **idle:** replace GetLastActivity RLock with lock-free atomic read ([e319a89](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e319a89bf393f97adf17cf69cd67ad4d14486652))
* **idle:** skip mutex in RecordActivity via atomic debounce check ([9bc90b3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9bc90b3168ccf795961d9e2c1b12047fdaa07420))
* **lock:** replace deadlock.RWMutex with sync.RWMutex on hot paths ([f3e3300](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f3e3300990a2411637b900c14e67a5345b9f653a))
* **log:** demote hot-path log.Info to log.Debug in checkSession ([43182e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43182e8c39f45faa65e09960a728d65929704713))
* **pool:** eliminate 4KB alloc on status-cache miss via sync.Pool ([7c21eee](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7c21eee6250d4779fee01f90661581f6263ca10d))
* **pty:** remove outer RLock from buffer accessors on immutable field ([346cffe](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/346cffe4aa4ef96be17e68208b0a67f71c2f8f10))
* replace InfoLog.Printf with log.Debug in hot determiner path ([06b7279](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/06b7279d1307b7cade83185ae872e0cd88409adb))
* **server:** cache available programs at startup, not on every server-info request ([82b9cec](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/82b9cecf1cf5a8fd8f216cf1f4669622b189db5c))
* **session:** replace ControllerManager RWMutex with atomic.Pointer ([a1fe6f9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a1fe6f9208c7ba66aaabeadb75325d01a28ae63a))
* **session:** replace TmuxProcessManager session RWMutex with atomic.Pointer ([fbe7f3a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fbe7f3a7a67e7d72e5f6020f17062a7754da81f0))
* **snapshot:** skip buildSnapshot when terminal timestamps unchanged ([4c52474](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4c5247411a0b8197748ecf751a48b47eb15c6a3b))
* **status:** combine GetCurrentStatus+GetIdleState into one hash+cache read ([103e6a4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/103e6a4d169585825556d3cb502bdbc3e22df85a))
* **status:** replace InstanceStatusManager map+RWMutex with xsync.MapOf ([c7692fa](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c7692fa5dce141be4e16dc353e4d52f69b0176ca))
* **stream:** eliminate per-read mutex by reading exitTail from circular buffer ([97b91a1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/97b91a1742c081df78972bd41d00c4cecb7dac8f))
* **streamer:** replace byte-at-a-time ringBuffer loop with bulk copy ([2f5e906](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2f5e9068e080a9ee4fee49e027e27386ee5a1fdd))
* **stream:** skip ResponseChunk alloc in broadcast when no subscribers ([17669b9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/17669b9739fc11f82a3f669be948a03114d71ae1))
* **tmux:** atomic.Value+singleflight for DoesSessionExist; fix lock-held-across-I/O ([ef8d478](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ef8d4789806b9af267bb60ae0d7afec8de786f4d))
* **tmux:** eliminate scanner.Text() allocation on %output hot path ([e093c9b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e093c9bda5a5ce3090491675dcae0db70a8ae2b7))
* **tmux:** replace strconv.ParseUint with lookup table in decodeControlModeOutput ([88f9f6e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/88f9f6e7c397fe0193f096b7528ba2991a08cd39))
* **tmux:** use RLock in broadcastControlModeUpdate ([249dd63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/249dd638c79b7e95fe1667d02390307e500f18f3))
* variable dirty-cache TTL, eliminate N+1 metadata query, O(1) queue empty check ([dd2f088](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dd2f0882ab6a7fd07df0ddee0126b173751549fb))
* **vc:** cache GetBranch result 30s to eliminate per-RPC fork lock contention ([d3441f2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d3441f20012f5af464608031fe5a590cb604e346))
* **vcs:** cache VCSStatus per workdir for 15s to eliminate repeated git subprocesses ([33d3c8c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/33d3c8c9afc5aad6e4e7b31fe73e90ff262d10a4))
* **ws:** pool coalesce buffer to eliminate 1 more alloc/frame ([aea5933](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/aea59338802c9b1d9be9f497fe67ae5c3ff25013))
* **ws:** pool envelope buffer + marshal direct to avoid 2 allocs/frame ([2327d57](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2327d57b849ca62ad7e07ca94b2f8e3920005bc3))
* **ws:** replace snapshotCache map+RWMutex with xsync.MapOf ([e9baedd](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e9baedde5c62955f8379917da69ab2b56f1e1b7c))

## [1.35.0](https://github.com/tstapler/stapler-squad/compare/v1.34.0...v1.35.0) (2026-07-04)


### Features

* **agy,opencode:** add comprehensive CLI parity for Antigravity and OpenCode ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([38d8a29](https://github.com/tstapler/stapler-squad/commit/38d8a297b67d1fdbb13811a12ce0d7ddb5273222))
* **alias:** add name_prefix field + fix session name oscillation ([9c730be](https://github.com/tstapler/stapler-squad/commit/9c730bec10bbb84e7a8e3ab4a48015b542057602))
* **analytics:** bulk rule creation + page density improvements ([7ae2910](https://github.com/tstapler/stapler-squad/commit/7ae2910e41afd07b0ec1fa3495426a3f8a62d055))
* **analytics:** program detail panel + copilot review fixes ([2774602](https://github.com/tstapler/stapler-squad/commit/2774602a7b4f4cd89baf5d3f8abcabfa21e6a052))
* **analytics:** program detail panel with subcommand drill-down ([29c7af8](https://github.com/tstapler/stapler-squad/commit/29c7af8c25f81b935b048e8a23e27eab06e11a48))
* **analytics:** unify activity tables into single filterable view ([36de4b8](https://github.com/tstapler/stapler-squad/commit/36de4b80f50681cf33e070348eca34f40a457dd5))
* **approvals:** dismiss notifications when user sends terminal input ([#101](https://github.com/tstapler/stapler-squad/issues/101)) ([8a15753](https://github.com/tstapler/stapler-squad/commit/8a15753be0790197a5ffdcd6298fad3b32b15613))
* **artifacts:** extract PR URLs, commits & external URLs from JSONL history ([#129](https://github.com/tstapler/stapler-squad/issues/129)) ([bfe06a5](https://github.com/tstapler/stapler-squad/commit/bfe06a543ce070f5e87b7a501ccd0dda1a2221c2))
* **autonomous:** P0 foundation — AutonomousDriver + StatusChangeListener fan-out ([#105](https://github.com/tstapler/stapler-squad/issues/105)) ([95b7c69](https://github.com/tstapler/stapler-squad/commit/95b7c696df5ca3c0eea6f0e81d6c56ac664ba978))
* **autonomous:** triage migration, review queue UX, backlog MCP tools, session detail polish ([633e88d](https://github.com/tstapler/stapler-squad/commit/633e88d7b56d9bf8d9077c3f2109aadaf46b383a))
* **backlog:** finish GitHub sync — TriggerSync/GetSyncHistory RPCs, tests, settings UI ([#138](https://github.com/tstapler/stapler-squad/issues/138)) ([9c5927b](https://github.com/tstapler/stapler-squad/commit/9c5927b2551c8fced04723f010aba5424cec97b2))
* **backlog:** gate backlog behind feature flag on all layers ([66b1d83](https://github.com/tstapler/stapler-squad/commit/66b1d831c9d8bcc0df3d9ee654d2fd8822165283))
* **backlog:** gate backlog behind feature flag on all layers ([1186494](https://github.com/tstapler/stapler-squad/commit/1186494baa7d7f9429030f7703ad1e6e4b7e7132))
* **detection:** add StatusWaitingForAgent for background agent state ([#115](https://github.com/tstapler/stapler-squad/issues/115)) ([dddb62d](https://github.com/tstapler/stapler-squad/commit/dddb62d555e2752abfca5d25ce79c946580bf6aa))
* **detection:** detect monitors-still-running as WaitingForAgent ([#121](https://github.com/tstapler/stapler-squad/issues/121)) ([995d065](https://github.com/tstapler/stapler-squad/commit/995d065c81d059fbc7de93304efe56ca407af351))
* **detection:** fix session status indicators + observability ring buffer ([#110](https://github.com/tstapler/stapler-squad/issues/110)) ([f9f3079](https://github.com/tstapler/stapler-squad/commit/f9f30798eb9bb44a0da3a46f3593234061d347b0))
* **detection:** surface WAITING_FOR_AGENT as distinct SubStatus chip ([#126](https://github.com/tstapler/stapler-squad/issues/126)) ([e5f80e8](https://github.com/tstapler/stapler-squad/commit/e5f80e8bf4b691212b9524cfcab216caf5e3a43a))
* **events:** fix event pipeline gaps for cross-surface state consistency ([#120](https://github.com/tstapler/stapler-squad/issues/120)) ([016b558](https://github.com/tstapler/stapler-squad/commit/016b5584b5005363c7160e95181f307224264421))
* **files:** local file browser with iframe/SVG rendering ([#113](https://github.com/tstapler/stapler-squad/issues/113)) ([e55ada1](https://github.com/tstapler/stapler-squad/commit/e55ada1df4ed8a157b21949b74c9852823cf7e05))
* **files:** wire up the premium LocalFileBrowser component to the files page ([3743b94](https://github.com/tstapler/stapler-squad/commit/3743b945cb6085d160ed5734361b99442a66d9dc))
* gap-finder workflow + SESSION_STATUS_RESTORING ProtoToStatus fix ([#123](https://github.com/tstapler/stapler-squad/issues/123)) ([45a5a91](https://github.com/tstapler/stapler-squad/commit/45a5a91d53480f500e00fa584e4db7b115edf309))
* **harness:** headless triage test harness + alias kebab-case fix ([7ad3340](https://github.com/tstapler/stapler-squad/commit/7ad33400c273dcc3dc1ec6a210e3cdc4509fa2cf))
* **history:** Stories 3–5 — rich cards, virtual scroll, fork modal ([b61cfac](https://github.com/tstapler/stapler-squad/commit/b61cfacd68b2d07308c5ed38ce0ee5edffdb1a98))
* **history:** Story 1 — proto fields, git enrichment, fork dispatch ([711d5fe](https://github.com/tstapler/stapler-squad/commit/711d5fe0a84045407d877d0d60b4c4ad8ca92399))
* **insights:** time range filter, session detail, cost projections, perf ([#98](https://github.com/tstapler/stapler-squad/issues/98)) ([25e0653](https://github.com/tstapler/stapler-squad/commit/25e06533f12a0bf5df1096456f08bc384172c4ca))
* local file browser + detection readline fixes ([#116](https://github.com/tstapler/stapler-squad/issues/116)) ([5ce6c1c](https://github.com/tstapler/stapler-squad/commit/5ce6c1c0914c1e774b851f2ac8025a21fb7208f1))
* **nav:** group navigation into 4 sections, restore mobile access ([a2f78a3](https://github.com/tstapler/stapler-squad/commit/a2f78a38698a4a8414d1640a6b0c16795c527418))
* **omnibar:** add [@alias](https://github.com/alias) session presets ([#122](https://github.com/tstapler/stapler-squad/issues/122)) ([a65f7d2](https://github.com/tstapler/stapler-squad/commit/a65f7d2ebfd49239c5436f01e2978984dfa4d3bb))
* **omnibar:** alias name_prefix field and trust-dialog double-fire fix ([#128](https://github.com/tstapler/stapler-squad/issues/128)) ([7dbfb10](https://github.com/tstapler/stapler-squad/commit/7dbfb10c27524319ad63e918a3f4ea704b309818))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([670caef](https://github.com/tstapler/stapler-squad/commit/670caeffd64e6e89aaf13df26c692580aa4effe0))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([250e640](https://github.com/tstapler/stapler-squad/commit/250e64071b9ddb72d44ca85cd6f620988f6bfb33))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([bc794b7](https://github.com/tstapler/stapler-squad/commit/bc794b7fc4e88b39dbc86ece70a5c086e48d47b6))
* **pane:** add session peek modal triggered from pane picker ([9bf226a](https://github.com/tstapler/stapler-squad/commit/9bf226a7564e54b36b433cc52a359c573887cd14))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([837ba8c](https://github.com/tstapler/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **registry:** typed feature catalog replacing comment-based annotations ([#112](https://github.com/tstapler/stapler-squad/issues/112)) ([6232826](https://github.com/tstapler/stapler-squad/commit/6232826397e21f803d4af4bc01af481b7ac39545))
* **rules:** structured rule builder, template library, analytics→rule workflow ([#103](https://github.com/tstapler/stapler-squad/issues/103)) ([48adec6](https://github.com/tstapler/stapler-squad/commit/48adec6b2e47301106ea1b9192828fea445e83b2))
* **rules:** YAML import/export and UX improvements for ApprovalRulesPanel ([#123](https://github.com/tstapler/stapler-squad/issues/123)) ([ea92de7](https://github.com/tstapler/stapler-squad/commit/ea92de7fc83f8b060eabb2d5425c9f13233bc1cd))
* **server:** startup fast restore, terminal snapshot fix, build warnings ([#128](https://github.com/tstapler/stapler-squad/issues/128)) ([e158b30](https://github.com/tstapler/stapler-squad/commit/e158b305f3bb7b19e9a81e772f082140043df82a))
* **services:** extract CheckpointSvc, TerminalSvc, FeatureFlagSvc; delegate ListBranches+workflow (CDD Epic 1) ([8936b3a](https://github.com/tstapler/stapler-squad/commit/8936b3afc3ad039de23ee952e80979efdd45ca5a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([3571d03](https://github.com/tstapler/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** actor goroutine + mailbox + sendSync + goleak verification (IAC Epic 3) ([8adc0c9](https://github.com/tstapler/stapler-squad/commit/8adc0c9be7c2699fc506b365ba4f0bdd45e18790))
* **session:** add 30-min background reaper for paused-session tmux processes ([66e33e2](https://github.com/tstapler/stapler-squad/commit/66e33e24b88bf91153f1d4b935ce9cfdd63ff224))
* **session:** bidirectional session history transfer between Claude and Antigravity ([#130](https://github.com/tstapler/stapler-squad/issues/130)) ([08d3f87](https://github.com/tstapler/stapler-squad/commit/08d3f87b06a954acff5ab0a567defe97db407bed))
* **session:** enable/disable autonomous mode on existing sessions ([4f2967c](https://github.com/tstapler/stapler-squad/commit/4f2967cfb604880a4ede96e7e7c2cf0db90a10c3))
* **session:** IAC Epic 4 — actor-path state-machine core migration ([7bcc479](https://github.com/tstapler/stapler-squad/commit/7bcc479642a682671acd26d80cb618779614e418))
* **session:** InstanceSnapshot + atomic.Pointer read path, snapshot publish in all mutators (IAC Epic 1) ([e8cfafc](https://github.com/tstapler/stapler-squad/commit/e8cfafc44a56c217b4bfdb03a69b418ea8124ecd))
* **session:** migrate all unguarded readers to snapshot.Load() (IAC Epic 2) ([f491483](https://github.com/tstapler/stapler-squad/commit/f491483a76b7b5ff1fef2d6dc2131ff882ebe5ba))
* **session:** Registry + LiveInstance type-split lifecycle layer (IAC Epic 2.5) ([4232806](https://github.com/tstapler/stapler-squad/commit/42328063146596b88000b022ce0022f9e6341653))
* **sessions:** fix InitialPrompt injection, add session goal tracking MCP + UI ([#99](https://github.com/tstapler/stapler-squad/issues/99)) ([5132c3f](https://github.com/tstapler/stapler-squad/commit/5132c3fb4f7c1ae08bcb1b32cb4b510a9c7b948e))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([b695764](https://github.com/tstapler/stapler-squad/commit/b695764e9a5d12827da7db0815d81db262288dd9))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([f5f7f36](https://github.com/tstapler/stapler-squad/commit/f5f7f3618271b62783ca906274a5d4cac8eae13c))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([#124](https://github.com/tstapler/stapler-squad/issues/124)) ([e54f41b](https://github.com/tstapler/stapler-squad/commit/e54f41b012370f4f3620e2fe64cad122230fc66e))
* **status:** unify session status vocabulary across UI ([c42514d](https://github.com/tstapler/stapler-squad/commit/c42514d4020c41c6ad16ec2f546846e257a81563))
* support Antigravity CLI hooks.json format in ssq-hooks ([55df54d](https://github.com/tstapler/stapler-squad/commit/55df54d4f60be8cddf6082c8ed64621f60db26de))
* workflow run history, session archiving, slash commands, and pause memory optimization ([9c23e61](https://github.com/tstapler/stapler-squad/commit/9c23e6126664bc7faf458babbefc1fc7069e5c99))
* **workflows:** Quick Workflows — [@slug](https://github.com/slug) omnibar, management UI, cron scheduling ([#106](https://github.com/tstapler/stapler-squad/issues/106)) ([883deca](https://github.com/tstapler/stapler-squad/commit/883deca379d7529f921d654b6671c559c7766ab9))
* **workflows:** session affordances, retention enforcer, bulk-session RPCs ([ac4efa1](https://github.com/tstapler/stapler-squad/commit/ac4efa121abdb34649470cc16133e0a04872e20e))


### Bug Fixes

* address copilot review comments on analytics drill-down ([077f53f](https://github.com/tstapler/stapler-squad/commit/077f53f6e075ef467f61022a3be28321f56cc387))
* **alias:** default session type + name oscillation ([4d33c7d](https://github.com/tstapler/stapler-squad/commit/4d33c7d50c4cd66ed4cbbd585d9556329259f050))
* **approval:** inject PermissionRequest hook on CreateSession and RestartSession ([8238a64](https://github.com/tstapler/stapler-squad/commit/8238a64c57033833615cfa7dc83c26f2db57bd0b))
* **backlog:** address architecture-review follow-ups from post-merge audit ([#142](https://github.com/tstapler/stapler-squad/issues/142)) ([7791548](https://github.com/tstapler/stapler-squad/commit/7791548c1f3b633033975cdc4b2d6b44f0a7fc17))
* **backlog:** harden claude binary lookup against a stale Linux PATH ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([6839082](https://github.com/tstapler/stapler-squad/commit/68390826f0a22d380c04a1163c65017a113e8583))
* **backlog:** harden triage JSON parser against stray braces in preamble ([#134](https://github.com/tstapler/stapler-squad/issues/134)) ([fd56d6a](https://github.com/tstapler/stapler-squad/commit/fd56d6ae1e5fa73a86c42b2ef245c0ca7d10fba3))
* **backlog:** harden triage parser and add repoPath UI gate ([06f9857](https://github.com/tstapler/stapler-squad/commit/06f98573b53af59afab38dfb0684edcc59210531))
* **backlog:** replace idle triage sessions with headless pool calls ([#127](https://github.com/tstapler/stapler-squad/issues/127)) ([2d7e116](https://github.com/tstapler/stapler-squad/commit/2d7e116ca3f624a891babb2e27aa447e904c50be))
* **backlog:** TriggerTriage's cleanupCtx expired before it was ever used ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([6552ec9](https://github.com/tstapler/stapler-squad/commit/6552ec9039b1a9f364e874e552ecb9cea7f04530))
* **bench+lint:** gofmt PR[#101](https://github.com/tstapler/stapler-squad/issues/101) files; replace rebase with reset+recommit for baselines ([bf39276](https://github.com/tstapler/stapler-squad/commit/bf3927633606ae12218811e6a9d1c745a76f9682))
* **bench:** benchmark gate under 10 minutes — fix 3 root causes ([d7157f5](https://github.com/tstapler/stapler-squad/commit/d7157f50601c0a50ed86ce40068eff4c92e88ce4))
* **bench:** force-add gitignored baseline file; add retry + rebase on push ([f940c0a](https://github.com/tstapler/stapler-squad/commit/f940c0a12d19a69ee01da32cdd9c1bdf905d77a8))
* **bench:** make benchmark flows robust — 10 fragility fixes ([e69c0bd](https://github.com/tstapler/stapler-squad/commit/e69c0bd52580f61126edb779855ece3f05fd88b6))
* **ci:** add push retry loop to demo GIFs workflow ([e6a6f54](https://github.com/tstapler/stapler-squad/commit/e6a6f54a87b7925b725aff0bf3d524ee263795a5))
* **ci:** force-add bench-baseline.txt to bypass .gitignore in benchmark gate ([ebd4278](https://github.com/tstapler/stapler-squad/commit/ebd4278098639893694f192f457fb214ed481257))
* **ci:** resolve Benchmarks race, Demo GIFs 403, and Release Please body parse ([d30a2db](https://github.com/tstapler/stapler-squad/commit/d30a2db48a8790248239711c3e59015214242c26))
* **ci:** resolve pre-existing lint-css-tokens and lint-custom failures ([b959669](https://github.com/tstapler/stapler-squad/commit/b95966947df405db4525c270b8edd94b2a4d009a))
* **codesign:** auto-run setup-codesign and fix OpenSSL 3 PKCS12 compatibility ([de6c7a6](https://github.com/tstapler/stapler-squad/commit/de6c7a62ca8ddb61ce728d2094e59808bac8a3d6))
* **codesign:** install binary to ~/.stapler-squad/bin/ before signing to prevent sealed-resource invalidation ([6c539c3](https://github.com/tstapler/stapler-squad/commit/6c539c31202e181c93b5905d929d5a49067c6591))
* **db:** make approval_rule JSON fields Optional to fix SQLite migration ([3f06c42](https://github.com/tstapler/stapler-squad/commit/3f06c420517312b4d7bfed3ae5c4201a9c3c9c90))
* **deps:** upgrade gopsutil v3 to v4 to drop crashing go-m1cpu cgo init ([#129](https://github.com/tstapler/stapler-squad/issues/129)) ([69bc09f](https://github.com/tstapler/stapler-squad/commit/69bc09fa3564ecd7d965fc51386311c5b49bf79d))
* **detection:** detect dynamic workflows + expand turn-marker to ✦ ([2fad2c0](https://github.com/tstapler/stapler-squad/commit/2fad2c0eacc453b5a729b05d185774007622095f))
* **detection:** detect indented spinners and CR-overwritten esc-to-interrupt ([#108](https://github.com/tstapler/stapler-squad/issues/108)) ([54a5f63](https://github.com/tstapler/stapler-squad/commit/54a5f637a3859dc901d521bb3f9861032616ab63))
* **detection:** detect WaitingForAgent when esc-to-interrupt appears below spinner ([44b455a](https://github.com/tstapler/stapler-squad/commit/44b455a751cc3907a9ce3fdb3b2c945929ba6a25))
* **detection:** detect workflow approval dialog and idle-while-typing states ([63c8ca9](https://github.com/tstapler/stapler-squad/commit/63c8ca9cff55e40879a08954dcfd1c1a528c5dc1))
* **detection:** fix ⌛ Thinking chip missing on most active sessions ([375f7b0](https://github.com/tstapler/stapler-squad/commit/375f7b0419a0d2011aedc9576aeaab27a90e4710))
* **detection:** use bottom-up scan in review queue for no-controller sessions ([5d2ba8f](https://github.com/tstapler/stapler-squad/commit/5d2ba8f85acaa209f96ae3d6ab70d9d3ef49688a))
* **github:** prevent data race in UserPRCache.Start() via sync.Once ([9ec03fb](https://github.com/tstapler/stapler-squad/commit/9ec03fb5ce53102cc7243e8dfd946063f175e2bb))
* **headless:** update argsCapturingRunner.Run to match ClaudeRunner interface ([4678065](https://github.com/tstapler/stapler-squad/commit/4678065a591a625a39f3239e071c12e53b9e4b0a))
* **headless:** use Setsid instead of Noctty for headless runner subprocess ([095e09e](https://github.com/tstapler/stapler-squad/commit/095e09e33e2d01b9164cc5e0da707ed9fa83f35f))
* **history-linker:** preserve paused/hibernated session UUID on rescan ([#118](https://github.com/tstapler/stapler-squad/issues/118)) ([fb0e49f](https://github.com/tstapler/stapler-squad/commit/fb0e49f723385a12a77f47fa67157c02cd3790e3))
* **history:** Story 2 — fix broken resume (path validation + session type) ([5626e19](https://github.com/tstapler/stapler-squad/commit/5626e19cd9741881be63f432135ce6b8adf7ad37))
* **history:** virtual scroll layout + mark epic complete ([23f7ba0](https://github.com/tstapler/stapler-squad/commit/23f7ba00ed7214dabaf8e22d3c80dd5d7ea5a607))
* **install:** fix macOS FDA flow and health-check timeout on first install ([38c98b5](https://github.com/tstapler/stapler-squad/commit/38c98b518a383d4aee9076420d87382e5701f843))
* **install:** increase health-check timeout to 120s for session-restore latency ([f64eea0](https://github.com/tstapler/stapler-squad/commit/f64eea0bd4d55fd8b6cfee1def6bb0b2db253710))
* **install:** require launchctl bootstrap; remove launchctl load fallback ([9d3c503](https://github.com/tstapler/stapler-squad/commit/9d3c503ec531b78a5f5808623bdd1aecec1f7c49))
* **lint,status:** fix ESLint errors on main and complete status descriptions ([5635c56](https://github.com/tstapler/stapler-squad/commit/5635c567e7c2b5ce9606e15b42b5fbe166cda2ba))
* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([43110eb](https://github.com/tstapler/stapler-squad/commit/43110ebdf64d99b3ebe3bd6f1fc5adefeee190b9))
* **lint:** analytics-exempt line order; chore(ci): make demo GIFs manual-only ([1255f16](https://github.com/tstapler/stapler-squad/commit/1255f168a5ecbebc7c1d3ad88c58fdd1e0401711))
* **lint:** eslint-disable-next-line for forwardRef inside useMemo object ([5a956bb](https://github.com/tstapler/stapler-squad/commit/5a956bb5cea4eeaa84569e4b0e90adc184ef35c0))
* **lint:** gofmt rules_service_test.go ([cd3c4b1](https://github.com/tstapler/stapler-squad/commit/cd3c4b1628e9b8ffa6daf487f31695573b7bf8ec))
* **lint:** move useMemo above early returns in insights charts; add displayName ([902cab9](https://github.com/tstapler/stapler-squad/commit/902cab979f2b8ff9db584581fd36894d2d9e4452))
* **lint:** replace inline layout style with existing entryList class ([1a74ee5](https://github.com/tstapler/stapler-squad/commit/1a74ee534295c870bd73e8de670de205f952d6e7))
* **lint:** return empty map instead of nil in GetAllInstanceArtifacts ([c8b9780](https://github.com/tstapler/stapler-squad/commit/c8b97803aa46a02fa3b801ee073f1f45a2232a0e))
* **nav:** gate Backlog nav item behind feature flag ([3c2f739](https://github.com/tstapler/stapler-squad/commit/3c2f739a1e10790827c99d04d5080ae343a9b029))
* **notifications:** auto-resolve stale toasts and Chrome notifications ([#109](https://github.com/tstapler/stapler-squad/issues/109)) ([31fd160](https://github.com/tstapler/stapler-squad/commit/31fd160c667b018a817e60b2b8cb0853d831084d))
* **pane:** restore session peek modal integration in pane picker ([6aef840](https://github.com/tstapler/stapler-squad/commit/6aef84015c5dba709b17f812b8b1810abb601f5e))
* **registry:** add alias RPCs to scanner methodToID map ([a1ac791](https://github.com/tstapler/stapler-squad/commit/a1ac791b11dfa3d38dfa81ee56106221286b9987))
* **registry:** add workflow/detection RPCs to scanner methodToID map ([24388a0](https://github.com/tstapler/stapler-squad/commit/24388a079da33430e2d880e0b0647c8124b27388))
* repair main — duplicate actor declarations + orphaned config-file-rules stubs ([#140](https://github.com/tstapler/stapler-squad/issues/140)) ([9fa5059](https://github.com/tstapler/stapler-squad/commit/9fa5059d1da8090680499d35113a9cb912bcf049))
* **review-queue:** resolve UUID→Title before Remove so approved/deleted sessions leave the queue ([ea94659](https://github.com/tstapler/stapler-squad/commit/ea94659ff329737bc7a15fab2bfb9c927a2ed943))
* **review-queue:** show INPUT_REQUIRED items + UX improvements ([757666a](https://github.com/tstapler/stapler-squad/commit/757666aff79b654bcb8a52468ce7a26778ebeb11))
* **review-queue:** suppress auto-advance on session status transitions ([c3fa5e3](https://github.com/tstapler/stapler-squad/commit/c3fa5e3f523d070ccbada952a157a36b67206335))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/tstapler/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **rules:** correct coverage false-positives and add three-state UX ([#104](https://github.com/tstapler/stapler-squad/issues/104)) ([0d3d3af](https://github.com/tstapler/stapler-squad/commit/0d3d3af82156d45b4bec22d1321b7272c548275f))
* **server:** wire SessionHealthChecker into server startup ([bec3c4d](https://github.com/tstapler/stapler-squad/commit/bec3c4d65a13e9d4d7984dacc3b3a1264a86e2aa))
* **session:** hydrate SquadSessionID from legacy conversation_id key ([ac7e051](https://github.com/tstapler/stapler-squad/commit/ac7e051974558538de00e1f12a7dda8d1338196a))
* **session:** preserve initial prompt on restart, defer controller startup, add StartWithSize ([b4c78a1](https://github.com/tstapler/stapler-squad/commit/b4c78a15b056f1b22e6bf1ada07d32778f9ec488))
* **session:** program switching now saves correctly for all cases ([914138e](https://github.com/tstapler/stapler-squad/commit/914138ec162e494f9d90818182c93d3bb0a2ae53))
* **session:** release stateMutex before calling Start() in SwitchWorkspace ([f6f89d8](https://github.com/tstapler/stapler-squad/commit/f6f89d84868fc89e9242f6d3c68541548295dfbc))
* **session:** replace nil,nil return with ErrInstanceDataNotFound sentinel (nilnil lint) ([e2248ba](https://github.com/tstapler/stapler-squad/commit/e2248bac6bf0e8d8efc9a2ad7269dfad2a268525))
* **sessions:** prevent Claude process orphaning after server restart ([da222c2](https://github.com/tstapler/stapler-squad/commit/da222c259e1303a9b084489d550138fbb4539074))
* **session:** wire InitialPrompt to proto adapter and skip driver send when empty ([b8d9ed2](https://github.com/tstapler/stapler-squad/commit/b8d9ed297e9623b13a492dd23dafccd14555167f))
* **session:** wire status manager on new sessions before starting driver ([729968c](https://github.com/tstapler/stapler-squad/commit/729968c7b980f0132fa34c18db53e8459750ca72))
* **session:** wire status manager on new sessions before starting driver ([#111](https://github.com/tstapler/stapler-squad/issues/111)) ([715fcd5](https://github.com/tstapler/stapler-squad/commit/715fcd5ca4d6eda94f56aa016da8e45f88ee2e4a))
* **startup:** resume session drivers for sessions with undelivered InitialPrompt ([f62b772](https://github.com/tstapler/stapler-squad/commit/f62b77252ce8d3ff472ce95c4819a7fa6edfcc95))
* **status:** address second-pass UX review findings ([b03dc73](https://github.com/tstapler/stapler-squad/commit/b03dc7344430c8651c86cc34c6e07f9b23544ec4))
* **terminal:** prevent multi-tab disconnect from killing shared control-mode process ([#107](https://github.com/tstapler/stapler-squad/issues/107)) ([4314926](https://github.com/tstapler/stapler-squad/commit/43149267e596c515d481c1e7ac583eea37daf5fe))
* **terminal:** trailing-edge resize debounce + test isolation fixes ([#125](https://github.com/tstapler/stapler-squad/issues/125)) ([e5a42e7](https://github.com/tstapler/stapler-squad/commit/e5a42e730124cd557528e43a8f8b0c839377c162))
* **triage:** address code review findings from post-commit review ([6030616](https://github.com/tstapler/stapler-squad/commit/60306163562e4bf694580ad8b199c343db9fc0f1))
* **triage:** surface storage errors, add timeout, harden claude detection, link notifications to backlog ([59d41da](https://github.com/tstapler/stapler-squad/commit/59d41da13308eadabf3800e1279c4da11c011e28))
* **ui:** prevent N×5 dispatch storm freezing home page on load ([6d0859d](https://github.com/tstapler/stapler-squad/commit/6d0859d26265c6f4c11cf12f9e1a3371f3635090))
* **ux:** accessibility, status correctness, and detection bug fixes ([5dd16d5](https://github.com/tstapler/stapler-squad/commit/5dd16d57a98a6fd8bcf27ce5da0ee8ce96e178d2))
* **ux:** address 8 UX review findings from 2026-06-30 ([4780b94](https://github.com/tstapler/stapler-squad/commit/4780b94df04df4fc3334bd0d238a269601a95574))
* **ux:** session list accessibility and UX polish ([dee2af4](https://github.com/tstapler/stapler-squad/commit/dee2af4dd361d35bb1daa84b218e1b58da487513))
* **vcs:** periodically clear go-git repo cache to bound memory usage ([6a3e5dd](https://github.com/tstapler/stapler-squad/commit/6a3e5dd13811bd4416468ba67b0f656c385ac0f8))


### Performance Improvements

* batch HistoryLinker PIDs, guard hot debug logs, cache AheadBehind/CommitMessages, pool blob buffer ([#130](https://github.com/tstapler/stapler-squad/issues/130)) ([e385af2](https://github.com/tstapler/stapler-squad/commit/e385af280c449e5f940387214cc563c6412e2d33))
* **memory:** replace wt.Status() with index-based diff + customizable session columns ([1a147e0](https://github.com/tstapler/stapler-squad/commit/1a147e0d146b0c00064df0ab619a59bda37739e5))
* optimize CircularBuffer and eliminate full-buffer copies in status detection ([08a4ef7](https://github.com/tstapler/stapler-squad/commit/08a4ef75c2a7cf193823fc187c4573adc4189771))
* **session:** reduce lock contention across session concurrency primitives ([5dc5d8a](https://github.com/tstapler/stapler-squad/commit/5dc5d8ac524a6c1ad9fec28ad0f561101c6e924b))


### Reverts

* remove incorrect per-session hook injection in session_service ([cfdf4ff](https://github.com/tstapler/stapler-squad/commit/cfdf4ffb2d2ded1fb4604742da83f20ab44378c7))
* remove unnecessary commit-search-body flag ([35e0cd7](https://github.com/tstapler/stapler-squad/commit/35e0cd7894ae738e39492ac0e0955a98abd17f47))

## [1.34.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.1...v1.34.0) (2026-07-03)


### Features

* **backlog:** add hard delete for backlog items ([56483ca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/56483ca2ffa65f7b0b60c7a12d40f52df6481875))
* **backlog:** import backlog items from GitHub issues ([46cf6d4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46cf6d431f8604be9c645b7614278cf73f24bfdf))
* **perf:** add singleflight + hasUncommitted TTL cache to GoGitVCSReader ([6fc0fb2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6fc0fb20f187116698196946f82d707e3dc21f7a))
* **perf:** invalidate IsDirty cache on session Pause and Resume ([4cee38d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4cee38dc0ffd2f164ce19dad01e4fab7fb543955))


### Bug Fixes

* autonomous sessions rejected with "path is required" via omnibar ([#157](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/157)) ([5ab6c4a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ab6c4a08a03dbb6b28e5ae98975a72b543878d6))
* **nav:** add Feature Flags to navigation menu ([3fa665c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fa665cf20b250f51d09bb880fdcd9f1a7164c94))
* **perf:** release entry.mu before OS stat walk in HasUncommitted; typed nil returns in Do bodies ([71f52b3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/71f52b39bc907b69b7c6103db641ed59c6e92db0))
* **perf:** rename misleading panic test, add scope comment, move InvalidateDirtyCache post-transition ([799a352](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/799a3521a5a040772cf9aa23a071b4a84361eaf0))
* **service:** fall back to launchctl load when bootstrap fails on macOS ([46bd0ea](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46bd0ea9f5d92153f7bb279f56b2cbc0fcfdca56))
* **terminal:** correctly scan OSC/DCS escape sequences to stop render artifacts ([#156](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/156)) ([2151b4b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2151b4b2186627c64c1be1617e4e096918d5b491))

## [1.33.1](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.0...v1.33.1) (2026-07-02)


### Bug Fixes

* **analytics:** escape analytics session_id mismatch and dead mangle detection ([#149](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/149)) ([a121de8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a121de83717185a5a42183288ffde1f8e0a298cf))
* backlog/triage sessions die on launch (shell injection + flag-parsing crash) ([#150](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/150)) ([8016921](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8016921fa302c01ed9c16c5f568e410b9081b2b7))
* **backlog:** GitHub URL repo-path support, first-visit tour, and two related bugs ([#152](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/152)) ([6ef8164](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6ef8164b215f0c08c4a497b00fd33da516e9a288))
* web-build target doesn't generate proto bindings on a clean clone ([#155](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/155)) ([1bb135e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1bb135e8d93456c70db12ceb77b9f335369925f9))

## [1.33.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.32.0...v1.33.0) (2026-07-02)


### Features

* **pr-status:** show PR badge in row mode and use go-git for branch detection ([664fbfb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/664fbfb118d036a7ef52e0519a05d9e8368f22ca))


### Bug Fixes

* **codesign:** correct otool byte-order in verify-codesign plist decode ([ca60c0a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ca60c0ae893ba542a62cc46c5789af38fb57da4c))
* **css:** enable scroll on unfinished tab container ([fa5f37e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa5f37e25607ca094858655a57cbc0ca9202f368))
* **lint:** suppress norawexec on lookPathOnlyExecutor stub ([d054495](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d054495cb49d5a4151475df6fa0fe2e8cb99edc2))
* **lint:** use correct nolint directives for lookPathOnlyExecutor stub ([8987673](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/898767374dad24c5c6ab8e30aecf4ec5c96c5c29))
* repair broken release pipeline and build-from-source path ([#147](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/147)) ([1fc2ff6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1fc2ff6c90fe88af32f14fa600bdfc03a63fcf54))
* review queue auto-advance respects preference after approve/deny ([c63886a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c63886aa90a2354e41140a51280ad86eadfc06df))
* **unfinished:** stack GitHub auth banner vertically so Connect button is always visible ([750d252](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/750d252092abf455d7c20ad8ad143486e28674d2))

## [1.32.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.31.1...v1.32.0) (2026-07-01)


### Features

* **artifacts:** extract PR URLs, commits & external URLs from JSONL history ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([bfe06a5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bfe06a543ce070f5e87b7a501ccd0dda1a2221c2))
* **autonomous:** P0 foundation — AutonomousDriver + StatusChangeListener fan-out ([#105](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/105)) ([95b7c69](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/95b7c696df5ca3c0eea6f0e81d6c56ac664ba978))
* **autonomous:** triage migration, review queue UX, backlog MCP tools, session detail polish ([633e88d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/633e88d7b56d9bf8d9077c3f2109aadaf46b383a))
* **backlog:** implement CancelTriage RPC and session delete button ([a690c36](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a690c366d6704651b913a60dab6b8774303a7f45))
* **detection:** add StatusWaitingForAgent for background agent state ([#115](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/115)) ([dddb62d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dddb62d555e2752abfca5d25ce79c946580bf6aa))
* **detection:** detect monitors-still-running as WaitingForAgent ([#121](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/121)) ([995d065](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/995d065c81d059fbc7de93304efe56ca407af351))
* **detection:** fix session status indicators + observability ring buffer ([#110](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/110)) ([f9f3079](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f9f30798eb9bb44a0da3a46f3593234061d347b0))
* **detection:** surface WAITING_FOR_AGENT as distinct SubStatus chip ([#126](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/126)) ([e5f80e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5f80e8bf4b691212b9524cfcab216caf5e3a43a))
* **events:** fix event pipeline gaps for cross-surface state consistency ([#120](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/120)) ([016b558](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/016b5584b5005363c7160e95181f307224264421))
* **files:** local file browser with iframe/SVG rendering ([#113](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/113)) ([e55ada1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e55ada1df4ed8a157b21949b74c9852823cf7e05))
* gap-finder workflow + SESSION_STATUS_RESTORING ProtoToStatus fix ([#123](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/123)) ([45a5a91](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/45a5a91d53480f500e00fa584e4db7b115edf309))
* GitHub work continuity — persistence, annotation fallback, and type-safe RepoRef ([3be7e09](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3be7e0902f57295fb17953d9ff3a84cabcfd3b9f))
* GitHub work continuity — UserPRCache, GitHubUserService, and Unfinished Tab integration ([#141](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/141)) ([e322a7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e322a7ebf798ba9e18543fd9d84e396ad065d501))
* local file browser + detection readline fixes ([#116](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/116)) ([5ce6c1c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ce6c1c0914c1e774b851f2ac8025a21fb7208f1))
* **omnibar:** add [@alias](https://github.com/alias) session presets ([#122](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/122)) ([a65f7d2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a65f7d2ebfd49239c5436f01e2978984dfa4d3bb))
* **omnibar:** alias name_prefix field and trust-dialog double-fire fix ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([7dbfb10](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7dbfb10c27524319ad63e918a3f4ea704b309818))
* **onboarding:** offer to install Claude Code hooks during onboarding ([#138](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/138)) ([41e0206](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/41e0206f54acbf58203959fbc697e173cb7471fb))
* **pane:** add session peek modal triggered from pane picker ([9bf226a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9bf226a7564e54b36b433cc52a359c573887cd14))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/136)) ([837ba8c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **registry:** typed feature catalog replacing comment-based annotations ([#112](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/112)) ([6232826](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6232826397e21f803d4af4bc01af481b7ac39545))
* **rules:** auto-suggest rule name from criteria inputs ([#140](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/140)) ([d072abc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d072abc37fcb228dd052cd38cb8c324eb8c3bdcf))
* **rules:** structured rule builder, template library, analytics→rule workflow ([#103](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/103)) ([48adec6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/48adec6b2e47301106ea1b9192828fea445e83b2))
* **server:** startup fast restore, terminal snapshot fix, build warnings ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([e158b30](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e158b305f3bb7b19e9a81e772f082140043df82a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/137)) ([3571d03](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** add 30-min background reaper for paused-session tmux processes ([66e33e2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66e33e24b88bf91153f1d4b935ce9cfdd63ff224))
* **session:** enable/disable autonomous mode on existing sessions ([4f2967c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4f2967cfb604880a4ede96e7e7c2cf0db90a10c3))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([#124](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/124)) ([e54f41b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e54f41b012370f4f3620e2fe64cad122230fc66e))
* **status:** unify session status vocabulary across UI ([c42514d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c42514d4020c41c6ad16ec2f546846e257a81563))
* workflow run history, session archiving, slash commands, and pause memory optimization ([9c23e61](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9c23e6126664bc7faf458babbefc1fc7069e5c99))
* **workflows:** Quick Workflows — [@slug](https://github.com/slug) omnibar, management UI, cron scheduling ([#106](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/106)) ([883deca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/883deca379d7529f921d654b6671c559c7766ab9))
* **workflows:** session affordances, retention enforcer, bulk-session RPCs ([ac4efa1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ac4efa121abdb34649470cc16133e0a04872e20e))


### Bug Fixes

* **backlog:** replace idle triage sessions with headless pool calls ([#127](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/127)) ([2d7e116](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2d7e116ca3f624a891babb2e27aa447e904c50be))
* **ci:** force-add bench-baseline.txt to bypass .gitignore in benchmark gate ([ebd4278](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ebd4278098639893694f192f457fb214ed481257))
* **codesign:** auto-run setup-codesign and fix OpenSSL 3 PKCS12 compatibility ([de6c7a6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/de6c7a62ca8ddb61ce728d2094e59808bac8a3d6))
* **codesign:** install binary to ~/.stapler-squad/bin/ before signing to prevent sealed-resource invalidation ([6c539c3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6c539c31202e181c93b5905d929d5a49067c6591))
* **db:** make approval_rule JSON fields Optional to fix SQLite migration ([3f06c42](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3f06c420517312b4d7bfed3ae5c4201a9c3c9c90))
* **deps:** upgrade gopsutil v3 to v4 to drop crashing go-m1cpu cgo init ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([69bc09f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/69bc09fa3564ecd7d965fc51386311c5b49bf79d))
* **detection:** detect indented spinners and CR-overwritten esc-to-interrupt ([#108](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/108)) ([54a5f63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/54a5f637a3859dc901d521bb3f9861032616ab63))
* **detection:** detect WaitingForAgent when esc-to-interrupt appears below spinner ([44b455a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/44b455a751cc3907a9ce3fdb3b2c945929ba6a25))
* **detection:** detect workflow approval dialog and idle-while-typing states ([63c8ca9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/63c8ca9cff55e40879a08954dcfd1c1a528c5dc1))
* **detection:** fix ⌛ Thinking chip missing on most active sessions ([375f7b0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/375f7b0419a0d2011aedc9576aeaab27a90e4710))
* **detection:** use bottom-up scan in review queue for no-controller sessions ([5d2ba8f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5d2ba8f85acaa209f96ae3d6ab70d9d3ef49688a))
* **history-linker:** preserve paused/hibernated session UUID on rescan ([#118](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/118)) ([fb0e49f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb0e49f723385a12a77f47fa67157c02cd3790e3))
* **install:** fix macOS FDA flow and health-check timeout on first install ([38c98b5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38c98b518a383d4aee9076420d87382e5701f843))
* **install:** increase health-check timeout to 120s for session-restore latency ([f64eea0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f64eea0bd4d55fd8b6cfee1def6bb0b2db253710))
* **install:** require launchctl bootstrap; remove launchctl load fallback ([9d3c503](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d3c503ec531b78a5f5808623bdd1aecec1f7c49))
* **install:** skip FDA prompt for non-admin users with cert-signed binary ([c24d088](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c24d088cd1bdb72225f0ac81cb16846b7d637c2a))
* **lint,status:** fix ESLint errors on main and complete status descriptions ([5635c56](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5635c567e7c2b5ce9606e15b42b5fbe166cda2ba))
* **notifications:** auto-resolve stale toasts and Chrome notifications ([#109](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/109)) ([31fd160](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/31fd160c667b018a817e60b2b8cb0853d831084d))
* publish session update event on controller status change ([9241d3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9241d3e0bcb89c0f65971f64435f4afe4fab1064))
* **registry:** add workflow/detection RPCs to scanner methodToID map ([24388a0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/24388a079da33430e2d880e0b0647c8124b27388))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **rules:** correct coverage false-positives and add three-state UX ([#104](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/104)) ([0d3d3af](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0d3d3af82156d45b4bec22d1321b7272c548275f))
* **server:** wire SessionHealthChecker into server startup ([bec3c4d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bec3c4d65a13e9d4d7984dacc3b3a1264a86e2aa))
* **session-driver:** add live output check before prompt injection ([aa9834c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/aa9834cdc11b87460eb2d706946cc1c4e07eac2f))
* **session:** preserve initial prompt on restart, defer controller startup, add StartWithSize ([b4c78a1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4c78a15b056f1b22e6bf1ada07d32778f9ec488))
* **session:** wire InitialPrompt to proto adapter and skip driver send when empty ([b8d9ed2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b8d9ed297e9623b13a492dd23dafccd14555167f))
* **session:** wire status manager on new sessions before starting driver ([729968c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/729968c7b980f0132fa34c18db53e8459750ca72))
* **session:** wire status manager on new sessions before starting driver ([#111](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/111)) ([715fcd5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/715fcd5ca4d6eda94f56aa016da8e45f88ee2e4a))
* skip shell sourcing in test mode to prevent service test timeout ([49eb0ba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/49eb0ba5dab6438f9796f36fa6ff8da55b10875a))
* **startup:** resume session drivers for sessions with undelivered InitialPrompt ([f62b772](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f62b77252ce8d3ff472ce95c4819a7fa6edfcc95))
* **status:** address second-pass UX review findings ([b03dc73](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b03dc7344430c8651c86cc34c6e07f9b23544ec4))
* **terminal:** prevent multi-tab disconnect from killing shared control-mode process ([#107](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/107)) ([4314926](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43149267e596c515d481c1e7ac583eea37daf5fe))
* **terminal:** repair escape code pipeline for new Claude Code renderer ([#139](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/139)) ([dc71b82](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc71b828a6d6a61883cd12b7c666e836eda97c90))
* **terminal:** trailing-edge resize debounce + test isolation fixes ([#125](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/125)) ([e5a42e7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5a42e730124cd557528e43a8f8b0c839377c162))
* **tmux:** flush stale exists-cache in RestoreWithWorkDir and add --tmux-keep-server to plist ([12b992a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/12b992a32ca5e9997f6b042e719bc49ef99eec91))
* **triage:** address code review findings from post-commit review ([6030616](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/60306163562e4bf694580ad8b199c343db9fc0f1))
* **triage:** silent storage error, hung session timeout, and Claude detection false positives ([cc82510](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cc8251090c0d6987e8f439fb1ba7b8cbce5d41e7))
* **triage:** surface storage errors, add timeout, harden claude detection, link notifications to backlog ([59d41da](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/59d41da13308eadabf3800e1279c4da11c011e28))
* **ux:** accessibility, status correctness, and detection bug fixes ([5dd16d5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dd16d57a98a6fd8bcf27ce5da0ee8ce96e178d2))
* **ux:** session list accessibility and UX polish ([dee2af4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dee2af4dd361d35bb1daa84b218e1b58da487513))


### Performance Improvements

* batch HistoryLinker PIDs, guard hot debug logs, cache AheadBehind/CommitMessages, pool blob buffer ([#130](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/130)) ([e385af2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e385af280c449e5f940387214cc563c6412e2d33))
* **memory:** replace wt.Status() with index-based diff + customizable session columns ([1a147e0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1a147e0d146b0c00064df0ab619a59bda37739e5))
* optimize CircularBuffer and eliminate full-buffer copies in status detection ([08a4ef7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/08a4ef75c2a7cf193823fc187c4573adc4189771))
* **session:** reduce lock contention across session concurrency primitives ([5dc5d8a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dc5d8ac524a6c1ad9fec28ad0f561101c6e924b))
* **tmux:** add semaphore to cap concurrent capture-pane subprocesses ([fa54896](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa548964a70cc757ac46a5d49b810ca389b53f90))
* **vcs:** cache reachableSet results and batch-read blobs under single lock ([125f4ef](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/125f4ef7e3f32f0a1f05c213cd394e4ab2e5e3bd))

## [1.35.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.34.0...v1.35.0) (2026-07-07)


### Features

* **backlog:** enhance GitHub issue picker — PR support, timestamps, keyboard nav, history tier ([1e6e33e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1e6e33eea3b6db1cdc25feab74b3464a880b69c6))
* **backlog:** GitHub issue picker — browse repos and issues to import ([4157558](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4157558b1309879e570cb5a360288ca24fcaffde))
* build pinned tmux, wire TMUX_BIN into all test targets, add goroutine-dump TestMain and test-trace targets ([0fd79cd](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0fd79cdfb798567017d624ac222fa8526cb36bb6))
* wire tmux.Binary() into all call sites so TMUX_BIN env var is respected ([d9b188d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d9b188d1ecf382b8d37fda9c85e876a030853053))


### Bug Fixes

* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([3407f0c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3407f0c161e3132362be9f175118be02a692480a))
* **lint:** analytics-exempt line order; chore(ci): make demo GIFs manual-only ([d275e54](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d275e540c4935e6c06eba8257578eac8f3063c51))


### Performance Improvements

* drain responseCh non-blocking in waitForCommandOrDrain to reduce 6.6M block events ([6dc4552](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6dc45527eeb7b0c3721397c02cb08547a08bafee))
* **frontend:** virtualize session list card mode with react-virtuoso to eliminate 1845ms scroll task ([bad2867](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bad2867a448e08754e1ceb534604af86858b523b))
* singleflight coalescing for IsDirtyWithHint, go-git for IsBranchCheckedOut/getHeadCommitSHA; remove NOT STALE debug log ([f157fa8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f157fa8b09b329ced0ff41054b78fef9c5d3ed0a))

## [1.34.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.1...v1.34.0) (2026-07-03)


### Features

* **backlog:** add hard delete for backlog items ([56483ca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/56483ca2ffa65f7b0b60c7a12d40f52df6481875))
* **backlog:** import backlog items from GitHub issues ([46cf6d4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46cf6d431f8604be9c645b7614278cf73f24bfdf))
* **perf:** add singleflight + hasUncommitted TTL cache to GoGitVCSReader ([6fc0fb2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6fc0fb20f187116698196946f82d707e3dc21f7a))
* **perf:** invalidate IsDirty cache on session Pause and Resume ([4cee38d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4cee38dc0ffd2f164ce19dad01e4fab7fb543955))


### Bug Fixes

* autonomous sessions rejected with "path is required" via omnibar ([#157](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/157)) ([5ab6c4a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ab6c4a08a03dbb6b28e5ae98975a72b543878d6))
* **nav:** add Feature Flags to navigation menu ([3fa665c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fa665cf20b250f51d09bb880fdcd9f1a7164c94))
* **perf:** release entry.mu before OS stat walk in HasUncommitted; typed nil returns in Do bodies ([71f52b3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/71f52b39bc907b69b7c6103db641ed59c6e92db0))
* **perf:** rename misleading panic test, add scope comment, move InvalidateDirtyCache post-transition ([799a352](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/799a3521a5a040772cf9aa23a071b4a84361eaf0))
* **service:** fall back to launchctl load when bootstrap fails on macOS ([46bd0ea](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46bd0ea9f5d92153f7bb279f56b2cbc0fcfdca56))
* **terminal:** correctly scan OSC/DCS escape sequences to stop render artifacts ([#156](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/156)) ([2151b4b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2151b4b2186627c64c1be1617e4e096918d5b491))

## [1.33.1](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.0...v1.33.1) (2026-07-02)


### Bug Fixes

* **analytics:** escape analytics session_id mismatch and dead mangle detection ([#149](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/149)) ([a121de8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a121de83717185a5a42183288ffde1f8e0a298cf))
* backlog/triage sessions die on launch (shell injection + flag-parsing crash) ([#150](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/150)) ([8016921](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8016921fa302c01ed9c16c5f568e410b9081b2b7))
* **backlog:** GitHub URL repo-path support, first-visit tour, and two related bugs ([#152](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/152)) ([6ef8164](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6ef8164b215f0c08c4a497b00fd33da516e9a288))
* web-build target doesn't generate proto bindings on a clean clone ([#155](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/155)) ([1bb135e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1bb135e8d93456c70db12ceb77b9f335369925f9))

## [1.33.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.32.0...v1.33.0) (2026-07-02)


### Features

* **pr-status:** show PR badge in row mode and use go-git for branch detection ([664fbfb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/664fbfb118d036a7ef52e0519a05d9e8368f22ca))


### Bug Fixes

* **codesign:** correct otool byte-order in verify-codesign plist decode ([ca60c0a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ca60c0ae893ba542a62cc46c5789af38fb57da4c))
* **css:** enable scroll on unfinished tab container ([fa5f37e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa5f37e25607ca094858655a57cbc0ca9202f368))
* **lint:** suppress norawexec on lookPathOnlyExecutor stub ([d054495](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d054495cb49d5a4151475df6fa0fe2e8cb99edc2))
* **lint:** use correct nolint directives for lookPathOnlyExecutor stub ([8987673](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/898767374dad24c5c6ab8e30aecf4ec5c96c5c29))
* repair broken release pipeline and build-from-source path ([#147](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/147)) ([1fc2ff6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1fc2ff6c90fe88af32f14fa600bdfc03a63fcf54))
* review queue auto-advance respects preference after approve/deny ([c63886a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c63886aa90a2354e41140a51280ad86eadfc06df))
* **unfinished:** stack GitHub auth banner vertically so Connect button is always visible ([750d252](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/750d252092abf455d7c20ad8ad143486e28674d2))

## [1.32.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.31.1...v1.32.0) (2026-07-01)


### Features

* **artifacts:** extract PR URLs, commits & external URLs from JSONL history ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([bfe06a5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bfe06a543ce070f5e87b7a501ccd0dda1a2221c2))
* **autonomous:** P0 foundation — AutonomousDriver + StatusChangeListener fan-out ([#105](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/105)) ([95b7c69](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/95b7c696df5ca3c0eea6f0e81d6c56ac664ba978))
* **autonomous:** triage migration, review queue UX, backlog MCP tools, session detail polish ([633e88d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/633e88d7b56d9bf8d9077c3f2109aadaf46b383a))
* **backlog:** implement CancelTriage RPC and session delete button ([a690c36](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a690c366d6704651b913a60dab6b8774303a7f45))
* **detection:** add StatusWaitingForAgent for background agent state ([#115](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/115)) ([dddb62d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dddb62d555e2752abfca5d25ce79c946580bf6aa))
* **detection:** detect monitors-still-running as WaitingForAgent ([#121](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/121)) ([995d065](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/995d065c81d059fbc7de93304efe56ca407af351))
* **detection:** fix session status indicators + observability ring buffer ([#110](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/110)) ([f9f3079](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f9f30798eb9bb44a0da3a46f3593234061d347b0))
* **detection:** surface WAITING_FOR_AGENT as distinct SubStatus chip ([#126](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/126)) ([e5f80e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5f80e8bf4b691212b9524cfcab216caf5e3a43a))
* **events:** fix event pipeline gaps for cross-surface state consistency ([#120](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/120)) ([016b558](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/016b5584b5005363c7160e95181f307224264421))
* **files:** local file browser with iframe/SVG rendering ([#113](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/113)) ([e55ada1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e55ada1df4ed8a157b21949b74c9852823cf7e05))
* gap-finder workflow + SESSION_STATUS_RESTORING ProtoToStatus fix ([#123](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/123)) ([45a5a91](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/45a5a91d53480f500e00fa584e4db7b115edf309))
* GitHub work continuity — persistence, annotation fallback, and type-safe RepoRef ([3be7e09](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3be7e0902f57295fb17953d9ff3a84cabcfd3b9f))
* GitHub work continuity — UserPRCache, GitHubUserService, and Unfinished Tab integration ([#141](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/141)) ([e322a7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e322a7ebf798ba9e18543fd9d84e396ad065d501))
* local file browser + detection readline fixes ([#116](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/116)) ([5ce6c1c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ce6c1c0914c1e774b851f2ac8025a21fb7208f1))
* **omnibar:** add [@alias](https://github.com/alias) session presets ([#122](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/122)) ([a65f7d2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a65f7d2ebfd49239c5436f01e2978984dfa4d3bb))
* **omnibar:** alias name_prefix field and trust-dialog double-fire fix ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([7dbfb10](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7dbfb10c27524319ad63e918a3f4ea704b309818))
* **onboarding:** offer to install Claude Code hooks during onboarding ([#138](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/138)) ([41e0206](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/41e0206f54acbf58203959fbc697e173cb7471fb))
* **pane:** add session peek modal triggered from pane picker ([9bf226a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9bf226a7564e54b36b433cc52a359c573887cd14))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/136)) ([837ba8c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **registry:** typed feature catalog replacing comment-based annotations ([#112](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/112)) ([6232826](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6232826397e21f803d4af4bc01af481b7ac39545))
* **rules:** auto-suggest rule name from criteria inputs ([#140](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/140)) ([d072abc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d072abc37fcb228dd052cd38cb8c324eb8c3bdcf))
* **rules:** structured rule builder, template library, analytics→rule workflow ([#103](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/103)) ([48adec6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/48adec6b2e47301106ea1b9192828fea445e83b2))
* **server:** startup fast restore, terminal snapshot fix, build warnings ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([e158b30](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e158b305f3bb7b19e9a81e772f082140043df82a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/137)) ([3571d03](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** add 30-min background reaper for paused-session tmux processes ([66e33e2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66e33e24b88bf91153f1d4b935ce9cfdd63ff224))
* **session:** enable/disable autonomous mode on existing sessions ([4f2967c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4f2967cfb604880a4ede96e7e7c2cf0db90a10c3))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([#124](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/124)) ([e54f41b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e54f41b012370f4f3620e2fe64cad122230fc66e))
* **status:** unify session status vocabulary across UI ([c42514d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c42514d4020c41c6ad16ec2f546846e257a81563))
* workflow run history, session archiving, slash commands, and pause memory optimization ([9c23e61](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9c23e6126664bc7faf458babbefc1fc7069e5c99))
* **workflows:** Quick Workflows — [@slug](https://github.com/slug) omnibar, management UI, cron scheduling ([#106](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/106)) ([883deca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/883deca379d7529f921d654b6671c559c7766ab9))
* **workflows:** session affordances, retention enforcer, bulk-session RPCs ([ac4efa1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ac4efa121abdb34649470cc16133e0a04872e20e))


### Bug Fixes

* **backlog:** replace idle triage sessions with headless pool calls ([#127](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/127)) ([2d7e116](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2d7e116ca3f624a891babb2e27aa447e904c50be))
* **ci:** force-add bench-baseline.txt to bypass .gitignore in benchmark gate ([ebd4278](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ebd4278098639893694f192f457fb214ed481257))
* **codesign:** auto-run setup-codesign and fix OpenSSL 3 PKCS12 compatibility ([de6c7a6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/de6c7a62ca8ddb61ce728d2094e59808bac8a3d6))
* **codesign:** install binary to ~/.stapler-squad/bin/ before signing to prevent sealed-resource invalidation ([6c539c3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6c539c31202e181c93b5905d929d5a49067c6591))
* **db:** make approval_rule JSON fields Optional to fix SQLite migration ([3f06c42](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3f06c420517312b4d7bfed3ae5c4201a9c3c9c90))
* **deps:** upgrade gopsutil v3 to v4 to drop crashing go-m1cpu cgo init ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([69bc09f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/69bc09fa3564ecd7d965fc51386311c5b49bf79d))
* **detection:** detect indented spinners and CR-overwritten esc-to-interrupt ([#108](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/108)) ([54a5f63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/54a5f637a3859dc901d521bb3f9861032616ab63))
* **detection:** detect WaitingForAgent when esc-to-interrupt appears below spinner ([44b455a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/44b455a751cc3907a9ce3fdb3b2c945929ba6a25))
* **detection:** detect workflow approval dialog and idle-while-typing states ([63c8ca9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/63c8ca9cff55e40879a08954dcfd1c1a528c5dc1))
* **detection:** fix ⌛ Thinking chip missing on most active sessions ([375f7b0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/375f7b0419a0d2011aedc9576aeaab27a90e4710))
* **detection:** use bottom-up scan in review queue for no-controller sessions ([5d2ba8f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5d2ba8f85acaa209f96ae3d6ab70d9d3ef49688a))
* **history-linker:** preserve paused/hibernated session UUID on rescan ([#118](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/118)) ([fb0e49f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb0e49f723385a12a77f47fa67157c02cd3790e3))
* **install:** fix macOS FDA flow and health-check timeout on first install ([38c98b5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38c98b518a383d4aee9076420d87382e5701f843))
* **install:** increase health-check timeout to 120s for session-restore latency ([f64eea0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f64eea0bd4d55fd8b6cfee1def6bb0b2db253710))
* **install:** require launchctl bootstrap; remove launchctl load fallback ([9d3c503](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d3c503ec531b78a5f5808623bdd1aecec1f7c49))
* **install:** skip FDA prompt for non-admin users with cert-signed binary ([c24d088](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c24d088cd1bdb72225f0ac81cb16846b7d637c2a))
* **lint,status:** fix ESLint errors on main and complete status descriptions ([5635c56](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5635c567e7c2b5ce9606e15b42b5fbe166cda2ba))
* **notifications:** auto-resolve stale toasts and Chrome notifications ([#109](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/109)) ([31fd160](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/31fd160c667b018a817e60b2b8cb0853d831084d))
* publish session update event on controller status change ([9241d3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9241d3e0bcb89c0f65971f64435f4afe4fab1064))
* **registry:** add workflow/detection RPCs to scanner methodToID map ([24388a0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/24388a079da33430e2d880e0b0647c8124b27388))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **rules:** correct coverage false-positives and add three-state UX ([#104](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/104)) ([0d3d3af](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0d3d3af82156d45b4bec22d1321b7272c548275f))
* **server:** wire SessionHealthChecker into server startup ([bec3c4d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bec3c4d65a13e9d4d7984dacc3b3a1264a86e2aa))
* **session-driver:** add live output check before prompt injection ([aa9834c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/aa9834cdc11b87460eb2d706946cc1c4e07eac2f))
* **session:** preserve initial prompt on restart, defer controller startup, add StartWithSize ([b4c78a1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4c78a15b056f1b22e6bf1ada07d32778f9ec488))
* **session:** wire InitialPrompt to proto adapter and skip driver send when empty ([b8d9ed2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b8d9ed297e9623b13a492dd23dafccd14555167f))
* **session:** wire status manager on new sessions before starting driver ([729968c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/729968c7b980f0132fa34c18db53e8459750ca72))
* **session:** wire status manager on new sessions before starting driver ([#111](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/111)) ([715fcd5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/715fcd5ca4d6eda94f56aa016da8e45f88ee2e4a))
* skip shell sourcing in test mode to prevent service test timeout ([49eb0ba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/49eb0ba5dab6438f9796f36fa6ff8da55b10875a))
* **startup:** resume session drivers for sessions with undelivered InitialPrompt ([f62b772](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f62b77252ce8d3ff472ce95c4819a7fa6edfcc95))
* **status:** address second-pass UX review findings ([b03dc73](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b03dc7344430c8651c86cc34c6e07f9b23544ec4))
* **terminal:** prevent multi-tab disconnect from killing shared control-mode process ([#107](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/107)) ([4314926](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43149267e596c515d481c1e7ac583eea37daf5fe))
* **terminal:** repair escape code pipeline for new Claude Code renderer ([#139](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/139)) ([dc71b82](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc71b828a6d6a61883cd12b7c666e836eda97c90))
* **terminal:** trailing-edge resize debounce + test isolation fixes ([#125](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/125)) ([e5a42e7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5a42e730124cd557528e43a8f8b0c839377c162))
* **tmux:** flush stale exists-cache in RestoreWithWorkDir and add --tmux-keep-server to plist ([12b992a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/12b992a32ca5e9997f6b042e719bc49ef99eec91))
* **triage:** address code review findings from post-commit review ([6030616](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/60306163562e4bf694580ad8b199c343db9fc0f1))
* **triage:** silent storage error, hung session timeout, and Claude detection false positives ([cc82510](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cc8251090c0d6987e8f439fb1ba7b8cbce5d41e7))
* **triage:** surface storage errors, add timeout, harden claude detection, link notifications to backlog ([59d41da](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/59d41da13308eadabf3800e1279c4da11c011e28))
* **ux:** accessibility, status correctness, and detection bug fixes ([5dd16d5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dd16d57a98a6fd8bcf27ce5da0ee8ce96e178d2))
* **ux:** session list accessibility and UX polish ([dee2af4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dee2af4dd361d35bb1daa84b218e1b58da487513))


### Performance Improvements

* batch HistoryLinker PIDs, guard hot debug logs, cache AheadBehind/CommitMessages, pool blob buffer ([#130](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/130)) ([e385af2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e385af280c449e5f940387214cc563c6412e2d33))
* **memory:** replace wt.Status() with index-based diff + customizable session columns ([1a147e0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1a147e0d146b0c00064df0ab619a59bda37739e5))
* optimize CircularBuffer and eliminate full-buffer copies in status detection ([08a4ef7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/08a4ef75c2a7cf193823fc187c4573adc4189771))
* **session:** reduce lock contention across session concurrency primitives ([5dc5d8a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dc5d8ac524a6c1ad9fec28ad0f561101c6e924b))
* **tmux:** add semaphore to cap concurrent capture-pane subprocesses ([fa54896](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa548964a70cc757ac46a5d49b810ca389b53f90))
* **vcs:** cache reachableSet results and batch-read blobs under single lock ([125f4ef](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/125f4ef7e3f32f0a1f05c213cd394e4ab2e5e3bd))

## [1.31.1](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.31.0...v1.31.1) (2026-06-02)


### Bug Fixes

* **lint:** gofmt rules_service_test.go ([cd3c4b1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cd3c4b1628e9b8ffa6daf487f31695573b7bf8ec))

## [1.31.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.30.0...v1.31.0) (2026-06-02)


### Features

* add support for Antigravity (agy) CLI ([a981bcb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a981bcb16db6ee557c9a487b48703951c48d2cdd))
* **agy:** full Antigravity CLI support — hooks, install, UI, detection ([e66d058](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e66d05839789737cabad6d8b9401c3b431aff9e8))
* **analytics:** program detail panel with subcommand drill-down ([#85](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/85)) ([9d43582](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d4358272d7293457c6f6474480d66f4bd4b2341))
* **backlog:** automated triage pipeline with review panel ([fcafaed](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fcafaedd9bdb2a01e8765d44d9d7324df0a80ec8))
* **backlog:** session monitor panel and backlog detail improvements ([80d3f3b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/80d3f3bf60fbb949c549265055a9855e43a631b6))
* **backlog:** triage re-trigger fix, detail UI polish, backlog in mobile nav ([6a174c3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6a174c3a0e1298bec2cc387b31de51f5c0eb7ecd))
* **backlog:** workflow engine and status event audit log ([7458bcb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7458bcb33d42d98fa2602c8caaab44cd402b5106))
* **headless:** session/headless pool — cache-optimized background LLM calls via claude -p ([fba787e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fba787e5c843848fceb76eb93654bf03ab8748a0))
* **headless:** session/headless pool — cache-optimized background LLM calls via claude -p ([0a4e10e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0a4e10e7714c495375468a46c5da7bc44d712b87))
* **mcp:** tag MCP-created sessions with source:mcp ([e3191bc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e3191bc271dbac302c944ee95e6c7f2ef5aaf36d))
* **memory:** surface memory pressure and fix hibernation sweeper ([f455f3b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f455f3bf655eea20690c3a5cdfdcb8ce5c132d5f))
* **memory:** surface memory pressure and fix hibernation sweeper ([5632d7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5632d7eacfcc47742b300b7b8ad097d7f0e0ed83))
* **memory:** surface memory pressure and fix hibernation sweeper ([2927de7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2927de74739889a092d796e11b19190d7516ba2f))
* **rules:** modal dialog, scrollable table, analytics deep-link pre-fill ([6f654e3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6f654e30e9b721c8ab1f36dc6522a38093bc56dc))
* **rules:** UX improvements — toast z-index, mobile layout, form sections, table clarity ([17b2a6f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/17b2a6f20d4ef2e110c11d2356bc60d42c46eb14))
* **rules:** YAML import/export and UX improvements for ApprovalRulesPanel ([#123](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/123)) ([ea92de7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ea92de7fc83f8b060eabb2d5425c9f13233bc1cd))
* **session:** immortal migration — ProcessManager interface + TmuxBackend + NativeProcessManager ([#113](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/113)) ([66b4c65](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66b4c65179ce444bcedb14f97af87d1cb90020fb))
* **session:** session steering — supervised driver for all automated sessions ([874c1e6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/874c1e63409953706378430326570fc2491112d4))
* **session:** WriteToSession RPC for sending raw input to a PTY ([5b528f1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5b528f1a015d202412b10084d0c41553f42d9c19))
* **shell-tabs:** complete UX discovery, error states, and full test coverage ([#93](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/93)) ([c380068](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c380068e34ff3e6772d9e1b9a4ee8bf5c092f9ca))
* **shells:** custom shell tabs — interactive PTY shells attached to sessions ([#109](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/109)) ([fb5a6d1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb5a6d15d0e29b7e52cadf89412fac3f74ffc1c2))
* **upload:** multi-file + Android camera/gallery/file picker in terminal toolbar ([b4b43e4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4b43e4d04d3f9bc2281ae95765304873338e2ba))
* **upload:** multi-file + Android camera/gallery/file picker in terminal toolbar ([417ac0a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/417ac0a2c8821c18aa09f11cd7dcde0d021b5922))
* **ux:** paused session clarity — overlay, visual distinction, pause reason ([#90](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/90)) ([d07584c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d07584cd95b6fb7a5f630a9db019de8d457b1ec3))


### Bug Fixes

* address code review findings from history-linker and workspace-service ([e80cb2a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e80cb2aea4bd9369b68ca52b83bd856b462002a9))
* **backlog:** repair triage pipeline — prompt injection, session exit, and oneshot flag ([cbc938b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cbc938bd4927ba6c6145148a85ad8cf948be08ab))
* **ci:** add ent ORM generation step before lint and build ([81e7ebf](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/81e7ebfb627186e5e3feaf89ec317594df2a351c))
* **ci:** allow ESLint warnings in full-scan path (push/workflow_dispatch) ([94b273c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/94b273ceb21252dd87ee451ebc747b4ac08fcf0e))
* **ci:** include session/ent/ in build artifact for test and cross-compile jobs ([3d823f1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3d823f1169182cb82204649505610d67c77c668f))
* **detection:** detect ✻ asterism prefix as Claude active state ([#114](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/114)) ([a339b29](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a339b29c64117e3ec647f7a901a727562b79646a))
* **headless:** resolve all review findings — atomicity, security, concurrency, test coverage ([6f2cb76](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6f2cb76fb287820f34f38b9a5113eb89326c523d))
* **lint:** remove embedded ReviewState selector in sweeper test (QF1008) ([3fbcac9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fbcac9f25cee49d421a9be8e2332acc501f94a8))
* **lint:** remove redundant embedded field selector in sweeper test ([7b437d8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7b437d8037d873a8b2fe411cda950df9a50efc9f))
* **lint:** resolve prealloc violations and remove unused shellWgWait ([29c52c5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/29c52c5809efa20d0b830c3211850a61be95e161))
* **lint:** use LiveInstancesProvider and Active instead of deprecated LoadInstances/Running ([a99d967](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a99d967fdb023888e9d1fba4efb560a6fb82e597))
* **log:** prevent panic on service restart; fix ESLint violations ([38f666f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38f666f0665ba7bc042deb6f9d9a66c97a02e735))
* **macos:** stable TCC permission grants via self-signed codesigning ([#112](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/112)) ([3cc037f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3cc037fb0eaa3f151d2acfdc6ca04b72e5d7e113))
* **memory:** address code review findings — cache I/O, RSS coverage, error UX ([8cc95ba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8cc95ba0f6b5e93296af2549d0e84126619397f8))
* **memory:** fix cache population, pause guard, and error UX for hibernate/pause ([27b5584](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/27b5584bd421acdc03cbf0de811aa333f149eff6))
* **notifications:** condition-change gating for health alerts + native notification lifecycle ([#92](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/92)) ([c9b08c5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c9b08c5241bf6c48059df3f1fbd2beaa2db51228))
* **notifications:** fix type mapping, approval guard, and test enum values ([#110](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/110)) ([c4b2ee3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c4b2ee346b76c74cf38db3bb52ae273b74b0ee48))
* post-merge build repairs for sync/upstream-20260528 ([dea6a86](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dea6a869ca86668d147914fcd27b8c9a96899046))
* **session:** fix two data races and semaphore ordering from upstream merge ([5f3208a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5f3208a82425aa0323794faae1c968b4e60039f4))
* **session:** re-detect conversation UUID after /clear ([3ec93c4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3ec93c43ae0a307099b1f99c7592c02965d41bde))
* **session:** replace inst.tmuxManager.GetTmuxSessionName() with inst.GetTmuxSessionName() ([3651044](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3651044a4783502d1c17e664e57b0264653df4e7))
* **session:** replace time.Sleep with os.Chtimes in HistoryLinker tests to fix mtime race ([b731b6a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b731b6a560bb74580d7d5841ea95eea7010277e1))
* **session:** replace undefined NeedsApproval with detection layer check ([df56305](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/df563054c15be35f1983a7fc2b1baf969019e170))
* **sessions:** cascade session deletion to review queue, notifications, and modal auto-advance ([ce14e75](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ce14e7586aa48a1e3789a42d3adfb194575f8e10))
* **test:** use Stopped status in seedInstance to avoid tmux spawn in LoadInstances ([6b2f9d9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6b2f9d92ba2d9a9b7204ab54e17e6abae38a909b))
* **tokens:** use UTC in IsStale() to avoid timezone boundary false-positives ([b0cb29d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b0cb29d294271e7cbe1bceb7db300111a7d66b0c))
* **ui:** rename Gallery button to Images in terminal upload toolbar ([a23a124](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a23a124cbce4507c13001f7ffdf2e9df23b889d4))
* **ui:** show pane header tab strip on mobile (remove display:none at ≤768px) ([0eae9e6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0eae9e61b392cc6b96bb533e5cf6e5c2382bdb64))
* **upload:** extract FileList to array before input reset to fix Android upload ([c2ba4aa](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c2ba4aa156036a38143a0a029ff3c5845a635466))
* **upload:** fix stale upload tests and add disconnected-state UX fallback ([ebb8a1b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ebb8a1b3080babb41ef995c861d9386690f1ba02))


### Performance Improvements

* debounce omnibar Fuse searches and pre-sort sessions selector ([3231d75](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3231d75a5942fe77cc3bdf7d8cef99c90b125d15))
* eliminate hot-path allocs and reduce JS bundle size ([#107](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/107)) ([7e14c40](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7e14c407435b01f312038fac1b7b3a933e595a84))
* eliminate LoadInstances per-request in WorkspaceService and stagger git-status bursts ([dc99f6d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc99f6d79823bdc2c7c7ea56f52bcdd92b4c38dc))
* hot-path alloc elimination, go-git low-alloc index scan, JS bundle reduction ([#111](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/111)) ([14cc562](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/14cc562140030c42635d2071af0c61e3fd011dcf))
* reduce goroutine wakeups, lazy-load xterm.js, memoize SessionCard, direct SQL updates ([#122](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/122)) ([4104b01](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4104b01dec64aeecc91d7a49e2b09437b32b3204))
* remaining enforcement tests + CI coverage ([e8c150f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e8c150f5dee4a3b1928a401dde48165998203a03))
* **session:** lock-free GetTimeSinceLastMeaningfulOutput via atomic shadow ([22d1de5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/22d1de574786e04f038a38b0f11c103a4070b9fe))
* **session:** lock-free shell registry + allocation hot-path fixes ([8b883f3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8b883f37052af83299ef4f3752f2e487813a2f59))
* TTL cache for DiffShortstat + zero-alloc enforcement tests ([98d503c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/98d503c756cb2b71f0e5e374fc2af869ef1d1df6))

## [1.30.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.29.0...v1.30.0) (2026-05-18)


### Features

* **insights:** token & spend monitoring dashboard with model-over-time chart ([#104](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/104)) ([6b682cb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6b682cbc02f25dcda630cce13724b38f5d94b01a))

## [1.29.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.28.0...v1.29.0) (2026-05-18)


### Features

* **backlog:** add full backlog management layer ([#74](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/74)) ([765b904](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/765b9049729d80d14181a228d23205b1c622bdd1))
* **files:** resizable tree panel, mobile layout, recent files, quick-open palette ([#81](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/81)) ([78ecb19](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/78ecb19c9e547de968c2ed107964574aba0bcc68))
* **mobile:** mobile UX fixes, Go concurrency fixes, reflect-and-fix enforcement ([790d10c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/790d10c01ead2a5a9a3affd9bada79a10203fbed))
* **mobile:** pane header mobile fixes and session row layout improvements ([b4e841e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4e841e61814d6d1d195839031bf4cf9f0ff48d5))
* **ui:** dark slate theme, compact rows, onboarding modal, /help hub ([#79](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/79)) ([cf5b838](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cf5b8380a4a75cfc416aa8102271ca890c64c5c8))
* **upload:** generalize file upload to accept any file type with chip UI ([#80](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/80)) ([fb70cf3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb70cf315f4dd04156f2b0a5f6984c52ac7d923e))


### Bug Fixes

* **analytics:** serialize events as snake_case integers to fix null duration_ms ([0c2b35e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0c2b35ee534fc4cdfbd3331882983c193962b3d3))
* **mobile:** keyboard visible on foldable widths; restore overflow menu in row view ([13c7950](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/13c7950b6ae38024c261062ae0f6bd0a75bd7b4a))
* **sessions:** pass full action set to SessionRow in row view mode ([58d8aa3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/58d8aa3f44ebba1be81bb4c29e57907955c73ef0))
* **tests:** use meaningful alt text on FileChipList thumbnail to fix role queries ([e67f63d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e67f63d7f6c323bd8d795d654e28f5460bf528b6))
* **theme:** add missing statusDot and transition tokens to contract ([9f045cf](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9f045cfe2d8ba6813b1ce93a32896324384623b9))


### Performance Improvements

* eliminate forkExec lock contention from hot subprocess paths ([#102](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/102)) ([79582d4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/79582d49563347917065c73ee61d51dba5c9ac17))

## [1.28.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.27.0...v1.28.0) (2026-05-15)


### Features

* **analytics:** pluggable analytics system with SQLite storage and ESLint enforcement ([#69](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/69)) ([74f6f28](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/74f6f2819e1df1b64a4e0b45950a60dc7a0bfee0))
* **create-session:** opt-in to create directory + git repo when path is missing ([#43](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/43)) ([6e0ebe7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6e0ebe7fd0f56abfeb469c89055aaa2d5474d0c8))
* **events:** add sequence numbers and 1-hour catch-up replay to EventBus ([03bbfc7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/03bbfc7e0ac6158ee778bed206d021253c300ce9))
* **files:** PDF/video inline viewer and ranger-style keyboard navigation ([#71](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/71)) ([f753a86](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f753a86da0624aae2e2dfe1020cf35da9e134c03))
* **mobile:** add "+" pane button to mobile tab strip for split creation ([178d893](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/178d8937b729e083d099b826fda2c445d441726d))
* **pane:** add action bar with V/H keyboard shortcuts to pane picker ([9153f7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9153f7ef74dd81c2758b184a08c3f7945eba36d8))
* **pane:** keyboard-driven pane picker and smart session routing ([f7b437e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f7b437eba59bbc2661bdc675d1fbaa34bd99aa02))
* **pane:** open session in new pane via Alt+click, overflow menu, and omnibar button ([37cb863](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/37cb86374007afdff3b0f55f57ae711aa6ca1a33))
* **queue:** event-driven review queue — eliminate 2s polling lag for controller sessions ([#68](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/68)) ([c66db93](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c66db9383eda0f5ae5f8acdcc436429b68c99fd8))
* **terminal:** robust resize quiescence, scrollback, and mobile gestures ([#67](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/67)) ([57ae1c1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/57ae1c16fa8a1c681e4db7b3a2c04b6529ee2176))
* **ui:** replace emoji and unicode icons with Lucide components ([f97ed72](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f97ed72f5011ecebeed11953232c2f0103b7976e))
* **unfinished:** count untracked files in DiffShortstat; default to GoGitVCSReader ([d2fe92e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d2fe92e90a947b26677a96dc4feff3d94d1a8528))
* **unfinished:** VCSReader interface with CLI-git, go-git, and jj implementations ([e2c396c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e2c396ca7900747e9e75643219ab85304e84cac8))
* **ux:** cyberpunk theme system, cockpit layout & keyboard shortcuts ([#51](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/51)) ([e538e4b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e538e4b9e1a5e4121d5d6a448a8a0bc11d7b3dc6))
* **ux:** notifications page, nav badge, and smart pane targeting ([49bae61](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/49bae61ba9e349cb99b3cceaf968a478065f4f31))
* **ux:** tmux-style tiling pane engine with resizable splits ([8b45f92](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8b45f92dbdb1b45999fa4fa02d7b24285be016ce))
* **ux:** unified tmux-style tiling — any pane can show session list or detail ([c598ff1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c598ff16b193dfd59eedf27b6e1473455068c65e))
* **ux:** UX polish pass — session list split, toast, toolbar, card density ([123d24c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/123d24ce3f932b827bf4512cc7266b5a5b9e5a7c))


### Bug Fixes

* **a11y:** fix WCAG AA color-contrast violation in reset-layout button ([670fab2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/670fab2e9164e165cfb34320385c10b35ca3aa51))
* **a11y:** suppress card fade animation under prefers-reduced-motion ([d0377bd](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d0377bdd343f402e88d60266e7a848a99a05c91b))
* address is-it-ready review findings ([e038ef6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e038ef6b4beaed0967644b06fc511c96f17c6353))
* **analytics:** replace analytics-exempt with real track() calls in page.tsx ([ee0eb16](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ee0eb1661da746fd4e1859ed7e2be7116a7749d1))
* **build:** move vanilla-extract style to .css.ts file ([38a6806](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38a6806e06c4acd7d30b72635540a31cc74396c1))
* **ci:** resolve pre-existing benchmark path and analytics lint failures ([77e2d40](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/77e2d4086177143d60200dbe2ce62018862d65e3))
* correct bugs found in second is-it-ready pass ([c83e653](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c83e6531e0f8dfa7c09d46783f75e1a27e6ac1c4))
* **deps:** address PR review comments ([64eafd5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/64eafd5e47246e5822c8af9a92d25500255f0e91))
* **deps:** fix InstanceReader docstring and remove duplicate StateStore init ([a9b9b75](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a9b9b75366389b8d1952742fa05b9c425d1ac30b))
* **executor:** bound all blocking test reads/waits with 10s timeout helpers ([b309d96](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b309d96ef56d30218529e6f9101c0629a06e761b))
* **executor:** bound stderr test with context timeout to prevent hang ([45b7444](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/45b7444abd42e7c9da3002346f4c3277fbcdaee3))
* **frontend:** resolve TS type error in SessionDetailView + add planning docs ([43bbef3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43bbef364dba4e9c9666ff68e2ee0c7cdf68faa6))
* **lint:** resolve all pre-existing ESLint errors in full-scan mode ([b856ef8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b856ef8f3e3a974c9f22651350b466be03a366bd))
* **lint:** resolve all pre-existing ESLint warnings (react-hooks, a11y, next) ([b912a58](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b912a58456df500effcbb4a472fb08e1e9c07bd6))
* **lint:** update norawexec analyzer and add nolint directives to test helpers ([468a470](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/468a4709ebae20c454d0060b3338316c4d651453))
* **mobile:** comprehensive mobile UX improvements — pane switcher, toolbar, keyboard ([#61](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/61)) ([ec75da8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ec75da8e660751fbc436ea160f4bd0d1435f8ea6))
* **mobile:** restore scroll on sessions page by constraining leafContainer height ([b1d23e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b1d23e8ec27e3dd4825678dcc4aa94def726d91b))
* **mobile:** session navigation, list scrolling, and bottom nav height ([#63](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/63)) ([c669e74](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c669e74482a28dc2daca31d755f74fe4dba0c04d))
* **mobile:** show session tab bar on mobile when embedded in pane ([20f92d1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/20f92d15de6f1f2fb93b462fa0b512652f42e2e4))
* **nav:** show badge counts on icons when sidebar is collapsed ([#70](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/70)) ([e9582b8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e9582b81e87d91d84f07546019640007bd7648ca))
* **nav:** surface Notifications and Settings in desktop header bar ([4fc2a86](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4fc2a8697915d54da7957eee4f0fefdbc6d72fde))
* **nav:** unify DrawerNav with NAV_PAGES — add Notifications, Settings, Unfinished ([21be016](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/21be0165b8b8d0d99e6495e3f8f7333be4b30044))
* **notifications:** publish EventApprovalResponse to event bus for cross-device sync ([133067f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/133067f8db226d714e3b7e7f36c1051142c25055))
* **notifications:** resolve session title in approval and question notifications ([b6e2b57](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b6e2b5705a3fe210d20f4b4a42773d909d0f1989))
* **pane:** picker always shown with 2+ panes; sessions moveable between panes ([fd1e010](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fd1e0105762694580207f07705864341a4515718))
* **pane:** prevent double picker on sessions stream update ([339bb07](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/339bb07c82802085681096bd8d7d8bb41af67646))
* resolve all non-blocking review findings ([27602fb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/27602fb5829f97d4fd49d617fa83ef2d149f991d))
* **review-queue:** evaluate controller sessions on every poll cycle ([#72](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/72)) ([910cd19](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/910cd193bef6fbb511eb4917347c39faa3fa5183))
* **startup:** enforce tmux-before-sessions ordering with proof token ([94420fc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/94420fc92cc1b3ad8863691063957ae313cecc16))
* **startup:** wire notification store before subscriber + tiling pane fixes ([1523b0d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1523b0d3e2aed0152d72ce485d22feb65d89e452))
* **terminal:** correct snapshot rendering after resize ([3851540](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3851540dbe81cc839d260863df4bc79e04ac9b73))
* **terminal:** sync cursor position after snapshot replay and break resize oscillation ([347d991](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/347d991a581f6a059ffe9f584ae6dd2ae4ca7c19))
* **tests:** update type assertions, mock setup, and e2e fixtures ([c188efe](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c188efeaec144ade267ed0566552af5c9b5d7330))
* **test:** update makeStaleInstance to access cache via contentProvider ([ce627fb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ce627fb0631485a7aa8ea63e6828062ecec01f83))
* **types:** make isValidTab a proper type predicate for SessionDetailTab ([bb3f99a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bb3f99aada248cab6a69d67386072b0ff58c4d7c))
* **unfinished:** add --no-optional-locks to all scanner git commands to prevent index.lock contention ([28c1241](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/28c12412f310dff24061b24e05b869af9e12890b))
* **ux:** stop review queue page from spawning a competing WebSocket stream ([b1fff3d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b1fff3d65306c3c8ebd006e9274a72cfd7aaf717))


### Performance Improvements

* fix all hotpolllog violations from InfoLog extension (PerfFix-2/3/4) ([b81da4e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b81da4e07780b0686f81a58a0811cefd9eac4ec1))
* **log:** async writer eliminates log mutex contention in hot poll loop ([11f2d3c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/11f2d3c8b9e524e71757edf6dc62c1cfe192deae))

## [1.27.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.26.0...v1.27.0) (2026-05-06)


### Features

* **classifier:** safely parse multiline python -c blocks with # comments ([#97](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/97)) ([be05345](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/be05345332c65b431a656002f0223917ee0f3c47))
* **executor:** safe subprocess framework with zombie prevention and process group management ([4d03970](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4d03970402070b9bd841962fe25a0ac24df0e155))
* **lint:** add norawexec lint analyzer and safeexec framework plans ([cf96301](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cf963014f6d95290cbf94eac9371e8aa48f7ee77))
* **omnibar:** auto-populate session name, first prompt injection, inline shorthand ([#95](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/95)) ([58d94a7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/58d94a746d377fde1fb395bdca482747a1fc6bb7))


### Bug Fixes

* **control-mode:** eliminate 3s timeout race and blank terminal on dead session ([006b45a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/006b45a245ac6d5609166b946739ab13f856cb02))
* **executor:** resolve golangci-lint violations in new executor test files ([d6f154f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d6f154f7789dbc89d2193d523161f3be79a34ace))
* **session:** add WaitDelay to all missing subprocess calls to prevent zombie accumulation ([3224302](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/32243022110659eda4dbcbeea94a0464392e8dfc))

## [1.26.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.25.0...v1.26.0) (2026-05-05)


### Features

* **adr:** add ADR-010 frontend modularity + enforce with eslint-plugin-boundaries ([685eaba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/685eaba154332bacf854edb954d3ba3d6f7418e4))
* **classifier:** add approval rules, parser fixes, and CommandPattern linter ([#93](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/93)) ([a809681](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a809681d6a0fbe1672143cc942db780768a2fecf))
* **classifier:** auto-allow gh api PR review workflow commands ([#46](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/46)) ([e996dce](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e996dce6d66a28b245963e60dd22dbc9ed8d4cd7))
* **debug:** stream browser console logs to server via LogClientEvents RPC ([efac73a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/efac73abcf28c00b66f1b1dcc838a290528ba452))
* **engineering-excellence:** DI framework, error observability, CI hardening ([b2f692f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b2f692ff6384c834d2b81c1da8ec45bd4174b2bc))
* **engineering-excellence:** DI framework, error observability, CI hardening ([13ec9f3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/13ec9f3e5237fe26754ebab557a7d0d7edcb8a1e))
* **file-tree:** performance, themes, and file browser enhancements ([#86](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/86)) ([3f25b10](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3f25b10a2e2877bcc60d0cbfd522bc2d3be728bb))
* **lint:** add hotpolllog AST linter and remove stale debug logs ([e40e531](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e40e53165ee4eb0862436c006ade8572a78bd352))
* **log:** runtime log level control via REST API and debug menu ([34bcaf0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/34bcaf0d8c7298da708471828ce06a39a788d479))
* **mobile:** collapse secondary toolbar actions into ··· overflow menu ([ef342b6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ef342b6312fb09a34d891420612c0215fcad0bd1))
* **ratelimit:** detect Claude rate limits and auto-resume sessions ([#53](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/53)) ([ff5119f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ff5119f3d5bce43ac44779bf7daf6da4a54b609e))
* **ratelimit:** push-based output notification + repository improvements ([a0660b9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a0660b9636c973d748d220e20b750556f32491b7))
* **rules:** close coverage gaps found in approval analytics ([68602a4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/68602a4cab7b5f857d78e49fef5a3265f5adb03e))
* **session:** add ClearConversationState RPC + expand registry test coverage ([c7e9547](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c7e9547409c8d12e34bda6f9d4b6482bed5ad969))
* **session:** add New Project creation mode with git init ([#52](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/52)) ([e2e8964](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e2e89640ae5fef699ce4486fd4bdeb73315a30d6))
* **session:** persist review queue interaction state through restarts ([e22a4ca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e22a4ca4b821d7ca51847312d3a38ae8cc922c68))
* **ssq-hooks:** add claude install target and proper hook output format ([#45](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/45)) ([01d77ac](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/01d77ac93fa6481bab7c9117b6dba7e7ca0b39ed))
* **streaming:** WebSocket bridge for Watch* RPCs + global session context ([a374322](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a374322b9c7a9c618e8d9623dbc6ecf6f9268f27))
* **tmux:** priority CM sender for low-latency input forwarding ([93cfa92](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/93cfa924bced4a9442499463ed6112f4c73c3c11))
* **unfinished:** view diff modal + extract DiffRenderer as shared component ([47aa99f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/47aa99f8e94212aa2947934922e47b6fb7cb8ab7))


### Bug Fixes

* address PR review comments from Copilot ([d8aded5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d8aded55a3a98e4afd8dfdb6b27461b7e865e5ef))
* **adr-003:** eliminate all time.Sleep from test files; enforce with lint gate ([6c18a96](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6c18a965647c14796a74a5de10b8a96a48d79f50))
* **bench:** restore benchmark baselines from upstream-fanatics/main ([2aa57d7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2aa57d7d27398207519314c35c63e286ea70d296))
* **concurrency:** migrate mutexes to deadlock-detecting wrappers + zombie reaper refactor ([f8f012e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f8f012eb0f4f06a8bb722c7035c293b4b862bc31))
* **detection:** fix review queue misclassifying active sessions as idle ([#54](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/54)) ([d076dbe](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d076dbed01373a2488ecc914783e4caf09f69e24))
* **install:** build ssq-hooks to ~/.local/bin during make install ([3321fc5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3321fc5256673fa3805f2e36d81df2321638d881))
* **lint:** fix forbidigo violations in test watchdog helpers ([28645ac](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/28645aca7e0d6b18ef8ad01e14e9de933cd73932))
* **lint:** remove invalid _comment property from boundaries/dependencies rule ([3969fb4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3969fb4ac60e6556521a7d15ce9d593849acbb8f))
* **lint:** suppress gochecknoglobals on runtimeLevel atomic ([0058fad](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0058fad85a9498e230f64b76ecb9a63411afd3e5))
* **omnibar:** session name typing no longer resets one-off creation mode ([b1398cc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b1398cc0f0724a9d58efd4d52a45a10d47c76aff))
* **review:** address Copilot review comments ([6464477](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/64644771da87ea07f46e1afad16715654de0d38e))
* **review:** address remaining Copilot review comments ([9d25b4f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d25b4fa93f96f8c74564246eb8006766e22e0e3))
* **server:** use poller cache for WatchSessions; block on PTY pause ([18cd90f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/18cd90f2bbae9d310b4b9493258b79288b81154c))
* **session:** reduce zombie accumulation via WaitDelay and HistoryLinker backoff ([46756ef](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46756ef2a412d024381f4ceb8bf4df26e14acb44))
* **streaming:** replace polling with push-based updates for VCS and notifications ([14960de](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/14960dec217262ae8378a3795662023e983eaa24))
* **test:** prevent tmux server socket leaks across test runs ([7485594](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7485594cd1466a96b590b687a53e6db90f0a55e7))
* tmux session creation reliability and review queue force-advance ([a3d8655](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a3d865505fb239d522896d94a40ae230ea6b850e))
* **tmux:** initialize priority channels in CM dispatch test helper ([f9da30f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f9da30f765cf9bc202b9d6a233cb41cb2f5268de))
* **tmux:** initialize priority channels in CM dispatch test helper ([a7ae705](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a7ae705be731801ec561bc769b4bca08541e894f))
* **tmux:** non-blocking resize and fire-and-forget input to cut CM round-trips ([f0716d2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f0716d2133a3fe0a95a5518b051bcc8bc238e19a))
* **ui:** auto-reload once on ChunkLoadError to recover stale build cache ([3fd8ba2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fd8ba29aa9106b773f04e37244a4eb9f6d503af))
* **ui:** pre-populate worktree dropdown when repo is pre-selected ([784eac4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/784eac4caf254382caa6ce2f28334210e48cce91))
* **ui:** remove nav click handler that broke Next.js routing ([0cbeda8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0cbeda8f0b8b0a08e929ebd301ab2a83634bab09))
* **unfinished:** compare against remote tracking refs and add GetWorktreeDiff RPC ([dc605ad](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc605adcb1d208db902fd4baeba82ac62ce84205))


### Performance Improvements

* eliminate mutex contention and allocation hotspots ([#94](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/94)) ([6f3b278](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6f3b2781f5420d425741cf390f97ad3e94d1ce65))

## [1.25.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.24.0...v1.25.0) (2026-05-01)


### Features

* **zombie:** set PR_SET_CHILD_SUBREAPER on Linux so tmux's zombies reparent to us ([d597fd6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d597fd653dbbc7e24625a7e86fd2615125a3e3ef))


### Bug Fixes

* **ui:** show GitHub info in VCS tab; fix action sheet z-index ([9b95d93](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9b95d93c8a185bcd2c058f54c7399d5dceb6efc8))
* **zombie:** filter to direct children only, add spawn registry for origin logging ([a7d8fe2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a7d8fe27d17d6caf1ca3c8e473fb4b106fbbb123))
