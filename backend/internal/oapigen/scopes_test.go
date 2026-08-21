package oapigen_test

import (
	"slices"
	"testing"

	"multiagent-seo/internal/oapigen"
)

// publicOps are the two operations that are meant to be reachable without a
// login at all: the health probe and the login form itself.
var publicOps = []string{"GetHealthz", "Login"}

// knownRoles is the whole vocabulary the gate understands — it has to match
// user.Role, or a typo in a scope silently becomes "nobody may call this".
var knownRoles = []string{"admin", "advocate", "client", "guest"}

// TestEveryOperationDeclaresScopes is the guard the whole role split rests on.
// The auth middleware treats an empty scope list as admin-only, so a forgotten
// list fails closed rather than leaking — but it fails closed *silently*, and a
// new endpoint that quietly 403s for the advocate it was written for is its own
// kind of bug. Either way the spec has to say who an operation is for, out loud.
func TestEveryOperationDeclaresScopes(t *testing.T) {
	spec, err := oapigen.GetSwagger()
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if slices.Contains(publicOps, op.OperationID) {
				if op.Security != nil && len(*op.Security) > 0 {
					t.Errorf("%s %s (%s) is listed as public but declares security", method, path, op.OperationID)
				}
				continue
			}
			if op.Security == nil || len(*op.Security) == 0 {
				t.Errorf("%s %s (%s) declares no security — every operation must name its roles", method, path, op.OperationID)
				continue
			}
			for _, requirement := range *op.Security {
				scopes, ok := requirement["bearerAuth"]
				if !ok {
					t.Errorf("%s %s (%s) has a security requirement that is not bearerAuth", method, path, op.OperationID)
					continue
				}
				if len(scopes) == 0 {
					t.Errorf("%s %s (%s) has an empty role list — write [ admin ] (or the roles it is for) explicitly", method, path, op.OperationID)
				}
				for _, scope := range scopes {
					if !slices.Contains(knownRoles, scope) {
						t.Errorf("%s %s (%s) names unknown role %q", method, path, op.OperationID, scope)
					}
				}
			}
		}
	}
}

// TestAdvocateScopeIsOnlyOnMyRoutes pins the shape of the split: an advocate's
// token must not be accepted anywhere outside /my. Widening that is a decision,
// not an edit — this test is where it has to be made deliberately.
func TestAdvocateScopeIsOnlyOnMyRoutes(t *testing.T) {
	spec, err := oapigen.GetSwagger()
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Security == nil {
				continue
			}
			advocate := false
			for _, requirement := range *op.Security {
				if slices.Contains(requirement["bearerAuth"], "advocate") {
					advocate = true
				}
			}
			if advocate && !isMyRoute(path) {
				t.Errorf("%s %s (%s) lets an advocate in but is not under /my", method, path, op.OperationID)
			}
			if !advocate && isMyRoute(path) {
				t.Errorf("%s %s (%s) is under /my but shuts the advocate out", method, path, op.OperationID)
			}
		}
	}
}

func isMyRoute(path string) bool {
	return len(path) >= 4 && path[:4] == "/my/"
}

// TestClientScopesAreOnlyOnClientRoutes is the same pin as the advocate one, in
// the other direction: a launch-authenticated caller must not be admitted
// anywhere except the client's own section, and everything in that section must
// admit them — an admin token has no business there and would not carry a
// client id anyway.
func TestClientScopesAreOnlyOnClientRoutes(t *testing.T) {
	spec, err := oapigen.GetSwagger()
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Security == nil {
				continue
			}
			telegram := false
			for _, requirement := range *op.Security {
				scopes := requirement["bearerAuth"]
				if slices.Contains(scopes, "client") || slices.Contains(scopes, "guest") {
					telegram = true
				}
			}
			if telegram && !isClientRoute(path) {
				t.Errorf("%s %s (%s) lets a Telegram caller in but is not under /client", method, path, op.OperationID)
			}
			if !telegram && isClientRoute(path) {
				t.Errorf("%s %s (%s) is under /client but shuts the client out", method, path, op.OperationID)
			}
		}
	}
}

// TestGuestScopeIsOnlyWhereItHasTo pins the narrowest scope of all. A guest is a
// verified launch with no client behind it, so the only things it may reach are
// the picker and the intake that creates that client. Anything else would be
// reachable by anyone on Telegram.
func TestGuestScopeIsOnlyWhereItHasTo(t *testing.T) {
	allowed := []string{"ListClientSlots", "SubmitClientRequest"}

	spec, err := oapigen.GetSwagger()
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Security == nil {
				continue
			}
			for _, requirement := range *op.Security {
				if !slices.Contains(requirement["bearerAuth"], "guest") {
					continue
				}
				if !slices.Contains(allowed, op.OperationID) {
					t.Errorf("%s %s (%s) admits a guest — widening that is a decision, make it here",
						method, path, op.OperationID)
				}
			}
		}
	}
}

func isClientRoute(path string) bool {
	return len(path) >= 8 && path[:8] == "/client/"
}
