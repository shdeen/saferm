# SafeRM frontend

Source ownership follows these responsibilities:

| Directory                                                                   | Responsibility                                                             |
| --------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/app`                | Application composition, navigation, and help.                             |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/operations`         | History, details, status, results, and request hooks.                      |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/operations/dialogs` | One component per operation dialog, with shared submission props.          |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/configuration`      | Active and default configuration views.                                    |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/bridge`             | Calls to generated desktop services and their shared types.                |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/shared`             | Product copy and text formatting.                                          |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/styles`             | Application CSS.                                                           |
| `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/testing`            | Shared browser test setup; component tests remain beside their components. |

Run these commands from `/Users/shulemdeen/Docs Home/Coding/saferm/frontend`:

| Command                | Purpose                                                 |
| ---------------------- | ------------------------------------------------------- |
| `npm run dev`          | Starts the Vite frontend server.                        |
| `npm run build`        | Checks TypeScript and builds the production assets.     |
| `npm test`             | Runs the component tests.                               |
| `npm run format`       | Formats maintained frontend files with pinned Prettier. |
| `npm run format:check` | Checks formatting without changing files.               |

To launch the complete desktop application, run `wails3 task dev` from `/Users/shulemdeen/Docs Home/Coding/saferm`. The frontend server alone does not start the Go services.

Wails generates `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/bindings` from Go service declarations. Regenerate those files when service contracts change; keep handwritten bridge code in `/Users/shulemdeen/Docs Home/Coding/saferm/frontend/src/bridge`. Prettier excludes generated bindings and production assets.

Revised 2026-11-03
