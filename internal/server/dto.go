package server

import (
	"weft/internal/directory"
	"weft/internal/service"
)

// --- response DTOs ---

type meDTO struct {
	UID     string `json:"uid"`
	IsAdmin bool   `json:"isAdmin"`
	CSRF    string `json:"csrf"`
}

type posixDTO struct {
	UIDNumber     int    `json:"uidNumber"`
	GIDNumber     int    `json:"gidNumber"`
	HomeDirectory string `json:"homeDirectory"`
	LoginShell    string `json:"loginShell"`
	Gecos         string `json:"gecos,omitempty"`
}

type mailDTO struct {
	Mail    string   `json:"mail"`
	Aliases []string `json:"aliases,omitempty"`
}

type userDTO struct {
	UID         string            `json:"uid"`
	CN          string            `json:"cn"`
	SN          string            `json:"sn"`
	GivenName   string            `json:"givenName,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	POSIX       *posixDTO         `json:"posix,omitempty"`
	Mail        *mailDTO          `json:"mail,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

type groupDTO struct {
	CN        string   `json:"cn"`
	GIDNumber int      `json:"gidNumber"`
	MemberUID []string `json:"memberUid"`
}

func toUserDTO(u *directory.User) userDTO {
	d := userDTO{
		UID: u.UID, CN: u.CN, SN: u.SN,
		GivenName: u.GivenName, DisplayName: u.DisplayName,
	}
	if u.POSIX != nil {
		d.POSIX = &posixDTO{
			UIDNumber: u.POSIX.UIDNumber, GIDNumber: u.POSIX.GIDNumber,
			HomeDirectory: u.POSIX.HomeDirectory, LoginShell: u.POSIX.LoginShell,
			Gecos: u.POSIX.Gecos,
		}
	}
	if u.Mail != nil {
		d.Mail = &mailDTO{Mail: u.Mail.Mail, Aliases: u.Mail.Aliases}
	}
	d.Extra = u.Extra
	return d
}

func toUserDTOs(us []directory.User) []userDTO {
	out := make([]userDTO, len(us))
	for i := range us {
		out[i] = toUserDTO(&us[i])
	}
	return out
}

func toGroupDTO(g *directory.Group) groupDTO {
	m := g.MemberUID
	if m == nil {
		m = []string{}
	}
	return groupDTO{CN: g.CN, GIDNumber: g.GIDNumber, MemberUID: m}
}

func toGroupDTOs(gs []directory.Group) []groupDTO {
	out := make([]groupDTO, len(gs))
	for i := range gs {
		out[i] = toGroupDTO(&gs[i])
	}
	return out
}

// --- request DTOs ---

type posixReq struct {
	UIDNumber     int    `json:"uidNumber"`
	GIDNumber     int    `json:"gidNumber"`
	HomeDirectory string `json:"homeDirectory"`
	LoginShell    string `json:"loginShell"`
	Gecos         string `json:"gecos"`
	PrimaryGroup  string `json:"primaryGroup"`
}

func (p *posixReq) toInput() *service.POSIXInput {
	if p == nil {
		return nil
	}
	return &service.POSIXInput{
		UIDNumber: p.UIDNumber, GIDNumber: p.GIDNumber,
		HomeDirectory: p.HomeDirectory, LoginShell: p.LoginShell,
		Gecos: p.Gecos, PrimaryGroup: p.PrimaryGroup,
	}
}

type mailReq struct {
	Mail    string   `json:"mail"`
	Aliases []string `json:"aliases"`
}

func (m *mailReq) toProfile() *directory.MailProfile {
	if m == nil {
		return nil
	}
	return &directory.MailProfile{Mail: m.Mail, Aliases: m.Aliases}
}

type createUserReq struct {
	UID         string            `json:"uid"`
	CN          string            `json:"cn"`
	SN          string            `json:"sn"`
	GivenName   string            `json:"givenName"`
	DisplayName string            `json:"displayName"`
	Password    string            `json:"password"`
	POSIX       *posixReq         `json:"posix"`
	Mail        *mailReq          `json:"mail"`
	Extra       map[string]string `json:"extra"`
}

type updateUserReq struct {
	CN          string            `json:"cn"`
	SN          string            `json:"sn"`
	GivenName   string            `json:"givenName"`
	DisplayName string            `json:"displayName"`
	POSIX       *posixReq         `json:"posix"`
	Mail        *mailReq          `json:"mail"`
	Extra       map[string]string `json:"extra"`
}

// --- bulk import ---

// importRowReq is one user to create. Row is the client's index into the
// original file, echoed back so results can be correlated across chunks.
type importRowReq struct {
	Row int `json:"row"`
	createUserReq
}

type importReq struct {
	Rows []importRowReq `json:"rows"`
}

// importResultDTO reports the outcome of one row. Status is one of "created",
// "exists" (uid taken, row skipped), "invalid" (validation failed, batch
// continues), "error" (infrastructure failure; the rest of the chunk is
// "skipped").
type importResultDTO struct {
	Row    int      `json:"row"`
	UID    string   `json:"uid"`
	Status string   `json:"status"`
	Error  string   `json:"error,omitempty"`
	User   *userDTO `json:"user,omitempty"`
}

type importRespDTO struct {
	Results []importResultDTO `json:"results"`
}

type createGroupReq struct {
	CN        string `json:"cn"`
	GIDNumber int    `json:"gidNumber"`
}

type memberReq struct {
	UID string `json:"uid"`
}

type passwordReq struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type bootstrapReq struct {
	Password string `json:"password"` // the ldapd rootpw
}

// metaDTO exposes non-sensitive defaults so the SPA can pre-fill forms.
type metaDTO struct {
	BaseDN string `json:"baseDn"`
	// UserIDAttr is "uid" (default) or "cn": which LDAP attribute is the
	// user's naming/identifier attribute. The SPA hides the separate cn
	// display field and relabels the identity input when it's "cn", since
	// the two can't hold different values in that mode (see service.go).
	UserIDAttr    string `json:"userIdAttr"`
	PeopleOU      string `json:"peopleOu"`
	GroupsOU      string `json:"groupsOu"`
	PrimaryGroup  string `json:"primaryGroup"`
	DefaultShell  string `json:"defaultShell"`
	HomeTemplate  string `json:"homeTemplate"`
	UIDMin        int    `json:"uidMin"`
	UIDMax        int    `json:"uidMax"`
	GIDMin        int    `json:"gidMin"`
	GIDMax        int    `json:"gidMax"`
	MaxPwdLength  int    `json:"maxPasswordLength"`
	MailAttr      string `json:"mailAttr"`
	MailAliasAttr string `json:"mailAliasAttr"`
	// SessionTimeoutSeconds drives the SPA's inactivity auto-logout.
	SessionTimeoutSeconds int `json:"sessionTimeoutSeconds"`
	// UserAttrs describes the configured extra attributes so the SPA can
	// render their form fields generically.
	UserAttrs []userAttrDTO `json:"userAttrs"`
}

type userAttrDTO struct {
	Attr     string `json:"attr"`
	LabelDE  string `json:"labelDe"`
	LabelEN  string `json:"labelEn"`
	Required bool   `json:"required"`
}
