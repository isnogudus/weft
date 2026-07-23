package server

import (
	"context"
	"errors"
	"net/http"

	"weft/internal/directory"
	"weft/internal/service"
)

const (
	// maxImportRows caps the rows per import request. The client chunks well
	// below this (bcrypt time per row is the real cost); the cap only bounds a
	// single request's work.
	maxImportRows = 100
	// importMaxBodyBytes sizes the JSON body limit for maxImportRows rows with
	// aliases and extra attributes.
	importMaxBodyBytes = 2 << 20 // 2 MiB
)

// handleImportUsers creates the posted rows one by one, reusing the exact
// single-create pipeline per row (validation, bcrypt, id allocation under
// allocMu). LDAP has no transactions: rows are independent, nothing is rolled
// back, and the authoritative outcome is the per-row result list. The batch
// always answers 200 once it ran; only a malformed request is a 400.
func (s *Server) handleImportUsers(w http.ResponseWriter, r *http.Request) {
	var req importReq
	if err := readJSONMax(w, r, &req, importMaxBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	if len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "keine Zeilen")
		return
	}
	if len(req.Rows) > maxImportRows {
		writeError(w, http.StatusBadRequest, "zu viele Zeilen pro Anfrage (max. 100)")
		return
	}

	s.withConn(w, r, func(c directory.Conn) {
		results := make([]importResultDTO, 0, len(req.Rows))
		seen := make(map[string]bool, len(req.Rows))
		aborted := false
		for _, row := range req.Rows {
			res := importResultDTO{Row: row.Row, UID: row.UID}
			switch {
			case aborted:
				// An infrastructure failure hits every following row too; skip
				// them instead of burning a bcrypt hash per row. The client
				// resubmits error+skipped rows.
				res.Status = "skipped"
			case seen[row.UID]:
				res.Status = "invalid"
				res.Error = "doppelte uid in der Datei"
			default:
				seen[row.UID] = true
				res = s.importOne(r.Context(), c, row)
				if res.Status == "error" {
					aborted = true
				}
			}
			results = append(results, res)
		}
		writeJSON(w, http.StatusOK, importRespDTO{Results: results})
	})
}

func (s *Server) importOne(ctx context.Context, c directory.Conn, row importRowReq) importResultDTO {
	res := importResultDTO{Row: row.Row, UID: row.UID}

	// Cheap conflict pre-check so an existing uid costs no bcrypt hash. A
	// create racing past it still surfaces as ErrAlreadyExists below.
	switch _, err := c.GetUser(ctx, row.UID); {
	case err == nil:
		res.Status = "exists"
		return res
	case !errors.Is(err, directory.ErrNotFound):
		res.Status = "error"
		res.Error = err.Error()
		return res
	}

	u, err := s.svc.CreateUser(ctx, c, service.NewUser{
		UID: row.UID, CN: row.CN, SN: row.SN,
		GivenName: row.GivenName, DisplayName: row.DisplayName,
		Password: row.Password, POSIX: row.POSIX.toInput(), Mail: row.Mail.toProfile(),
		Extra: row.Extra,
	})
	switch {
	case err == nil:
		res.Status = "created"
		d := toUserDTO(u)
		res.User = &d
	case errors.Is(err, directory.ErrAlreadyExists):
		res.Status = "exists"
	case isDirError(err):
		res.Status = "error"
		res.Error = err.Error()
	default:
		res.Status = "invalid"
		res.Error = err.Error()
	}
	return res
}
