package gate

import "github.com/zhoudm1743/go-fast-framework/contracts"

// response 实现 contracts.GateResponse。
type response struct {
	allowed bool
	message string
}

func newResponse(allowed bool, message string) contracts.GateResponse {
	return &response{allowed: allowed, message: message}
}

func (r *response) Allowed() bool  { return r.allowed }
func (r *response) Denied() bool   { return !r.allowed }
func (r *response) Message() string { return r.message }
