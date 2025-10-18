package session

func ToSessionDTO(session Session) SessionModel {
	return SessionModel{
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		IDToken:      session.IDToken,
		ExpiresAt:    session.ExpiresAt,
	}
}

func ToSession(createSessionCommand CreateSessionCommand) Session {
	return Session{
		AccessToken:  createSessionCommand.AccessToken,
		RefreshToken: createSessionCommand.RefreshToken,
		IDToken:      createSessionCommand.IDToken,
		ExpiresAt:    createSessionCommand.ExpiresAt,
	}
}
