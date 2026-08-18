# Settings Cache Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure every system-settings page reloads authoritative saved values after immediate menu navigation, and ship the verified fix to production.

**Architecture:** Apply cache-lifecycle options once in the shared Settings query hook used by all six settings pages. Keep the existing page-level post-save refetches, and version the modified shared asset in the Service Worker precache manifest so existing browsers receive it.

**Tech Stack:** React Query compiled frontend assets, Workbox Service Worker, Go regression tests, Docker production runtime.

---

### Task 1: Add the static-bundle regression test

**Files:**
- Create: `src/route/settings_frontend_cache_test.go`

- [ ] **Step 1: Write a test that locates the shared Settings asset and asserts `staleTime:0`, `refetchOnMount:"always"`, and `gcTime:0`.**
- [ ] **Step 2: Assert that `src/www/sw.js` assigns that asset an explicit SHA-256 revision matching its file content.**
- [ ] **Step 3: Run `cd src && go test ./route -run TestSettingsFrontendCachePolicy -count=1 -v` and confirm it fails before the implementation.**

### Task 2: Implement the shared cache policy

**Files:**
- Modify: `src/www/assets/lib-DyUkl8FG.js`
- Modify: `src/www/sw.js`

- [ ] **Step 1: Pass `{staleTime:0,refetchOnMount:"always",gcTime:0}` as the Settings query options.**
- [ ] **Step 2: Calculate the modified asset SHA-256 and set it as the Service Worker precache revision.**
- [ ] **Step 3: Re-run `cd src && go test ./route -run TestSettingsFrontendCachePolicy -count=1 -v` and confirm it passes.**

### Task 3: Verify and publish

**Files:**
- Test: `src/route/settings_frontend_cache_test.go`
- Deploy: `src/www/assets/lib-DyUkl8FG.js`, `src/www/sw.js`

- [ ] **Step 1: Run `cd src && go test ./...` and require a zero exit code.**
- [ ] **Step 2: Review `git diff` and stage only the design, plan, regression test, shared asset, and Service Worker.**
- [ ] **Step 3: Commit and push the current branch without staging pre-existing database migration changes.**
- [ ] **Step 4: Back up the two production static files, copy the verified files, and confirm their hashes.**
- [ ] **Step 5: Reload the production page and verify save -> immediate menu switch -> return displays the saved value.**
- [ ] **Step 6: Confirm the public config endpoint remains healthy and returns the persisted EPay configuration.**
