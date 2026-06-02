package ctxutil

import "net/http"

func OrgID(r *http.Request) string {
	return r.Header.Get("X-Org-ID")
}

func HasOrgID(r *http.Request) bool {
	return r.Header.Get("X-Org-ID") != ""
}

func Role(r *http.Request) string {
	return r.Header.Get("X-User-Role")
}

func Subject(r *http.Request) string {
	return r.Header.Get("X-User-Subject")
}
