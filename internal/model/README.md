# model

`model` is the shared vocabulary for Go services. Connection, trust, session, and desktop pass these types for connection generations, resource targets, session references, and event envelopes.

An error or event that crosses a package boundary is a safe message. It does not carry credential values or raw helper output. The types are identities and envelopes. Kubernetes policy, client construction, and UI state stay in the packages that own those jobs.
