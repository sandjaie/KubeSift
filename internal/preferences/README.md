# preferences

`preferences` stores appearance and window geometry. Desktop `PreferencesService` is the caller. The file is a strict versioned document, written atomically. Geometry is clamped to logical bounds before it is saved.

`Load` returns an error for an unreadable file and does not rewrite it. A later `Save` of a valid document replaces the file. The desktop bridge skips background geometry saves while that load problem remains, until the user resets. The package does not import Wails. It does not store credentials, resource identities, Helm chart content, or session state. `helm.go` validates the Helm source metadata that is safe to persist: a bounded id and name, an HTTPS or OCI URL without userinfo, and optional CA, certificate, and key file paths. The file paths are references. Credential material and chart bytes are not stored.
