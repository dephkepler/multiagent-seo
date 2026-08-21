package oapigen_test

import (
	"slices"
	"testing"

	"multiagent-seo/internal/oapigen"
)

// publicOps are the two operations that are meant to be reachable without a
// login at all: the health probe and the login form itself.
var publicOps = []string{"GetHealthz", "Login"}

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
					if scope != "admin" && scope != "advocate" {
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
