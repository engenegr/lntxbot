package main

import (
	"database/sql"

	"github.com/go-telegram-bot-api/telegram-bot-api"
)

/*
   ALL GROUP CHAT TELEGRAM IDS ARE NEGATIVE
*/

type GroupChat struct {
	TelegramId int  `db:"telegram_id"`
	Spammy     bool `db:"spammy"`
	Ticket     int  `db:"ticket"`
}

type KickData struct {
	InvoiceMessage   tgbotapi.Message          `json:"invoice_message"`
	NotifyMessage    tgbotapi.Message          `json:"notify_message"`
	JoinMessage      tgbotapi.Message          `json:"join_message"`
	ChatMemberConfig tgbotapi.ChatMemberConfig `json:"chat_member_config"`
	NewMember        tgbotapi.User             `json:"new_member"`
	Hash             string                    `json:"hash"`
}

var spammy_cache = map[int64]bool{}

func toggleSpammy(telegramId int64) (spammy bool, err error) {
	err = pg.Get(&spammy, `
      INSERT INTO telegram.chat AS c (telegram_id, spammy) VALUES ($1, true)
      ON CONFLICT (telegram_id)
        DO UPDATE SET spammy = NOT c.spammy
        RETURNING spammy
    `, -telegramId)

	spammy_cache[-telegramId] = spammy

	return
}

func isSpammy(telegramId int64) (spammy bool) {
	if spammy, ok := spammy_cache[-telegramId]; ok {
		return spammy
	}

	err := pg.Get(&spammy, `
      SELECT spammy FROM telegram.chat WHERE telegram_id = $1
    `, -telegramId)
	if err != nil {
		return false
	}

	spammy_cache[-telegramId] = spammy

	return
}

func setTicketPrice(telegramId int64, sat int) (err error) {
	_, err = pg.Exec(`
      INSERT INTO telegram.chat AS c (telegram_id, ticket) VALUES ($1, $2)
      ON CONFLICT (telegram_id)
        DO UPDATE SET ticket = $2
        RETURNING spammy
    `, -telegramId, sat)
	return
}

func getTicketPrice(telegramId int64) (sat int, err error) {
	err = pg.Get(&sat, `
      SELECT ticket FROM telegram.chat WHERE telegram_id = $1
    `, -telegramId)
	if err == sql.ErrNoRows {
		return 0, nil
	}

	return
}
