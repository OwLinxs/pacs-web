package user

import (
	"time"
	_ "time/tzdata" // calendário municipal também no container mínimo
)

var accessLocation = func() *time.Location {
	l, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic("calendário de acesso indisponível")
	}
	return l
}()

// AccessDate é uma data de calendário, representada em UTC para transporte SQL DATE.
func AccessDate(now time.Time) time.Time {
	y, m, d := now.In(accessLocation).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
func AccessExpired(until *time.Time, now time.Time) bool {
	if until == nil {
		return false
	}
	y, m, d := until.Date()
	return AccessDate(now).After(time.Date(y, m, d, 0, 0, 0, 0, time.UTC))
}
