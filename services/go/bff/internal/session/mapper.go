package session

func ToSessionDTO(session Session) SessionModel {
	return SessionModel(session)
}

func ToSession(createSessionCommand CreateSessionCommand) Session {
	return Session{
		AccessToken:  createSessionCommand.AccessToken,
		RefreshToken: createSessionCommand.RefreshToken,
		IDToken:      createSessionCommand.IDToken,
		ExpiresAt:    createSessionCommand.ExpiresAt,
	}
}
