// Package desktop translates desktop calls to framework-independent services.
package desktop

type Service struct{}

func (*Service) Ping() string { return "ready" }
