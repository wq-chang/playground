package postgres

import (
	"github.com/guregu/null/v6"
	"github.com/jackc/pgx/v5/pgtype"
)

func ToPGText(n null.String) pgtype.Text {
	return pgtype.Text{
		String: n.String,
		Valid:  n.Valid,
	}
}
