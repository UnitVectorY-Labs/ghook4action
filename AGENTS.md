This is a Go application deployed through Docker. main.go lives at the repository root; implementation lives in internal/.

Keep README.md brief and document configuration, architecture, and operations in docs/.
Run Go tests and vet for changes. Subscriber changes need an integration test using an isolated Pub/Sub emulator and mock GitHub server; never pull a production subscription for testing.

Never log tokens or webhook bodies. Never execute repository content or follow URLs from webhook payloads. Do not add pull_request_target workflows. Project routing belongs in operator configuration, never application constants.
