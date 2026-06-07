package service

import "weft/internal/directory"

func directoryUser(uid string) directory.User {
	return directory.User{UID: uid, CN: uid, SN: uid}
}
