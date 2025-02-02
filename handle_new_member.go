package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fiatjaf/lightningd-gjson-rpc"
	"github.com/go-telegram-bot-api/telegram-bot-api"
)

var pendingApproval = make(map[string]KickData)

func handleNewMember(joinMessage *tgbotapi.Message, newmember tgbotapi.User) {
	sats, err := getTicketPrice(joinMessage.Chat.ID)
	if err != nil {
		log.Error().Err(err).Str("chat", joinMessage.Chat.Title).Msg("error fetching ticket price for chat")
		return
	}

	if sats == 0 {
		// no ticket policy
		return
	}

	// label for the invoice that will be shown
	label := fmt.Sprintf("newmember:%d:%d", newmember.ID, joinMessage.Chat.ID)

	if _, isPending := pendingApproval[label]; isPending {
		// user joined, left and joined again.
		// do nothing as the old timer is still counting.
		return
	}

	var username string
	if newmember.UserName != "" {
		username = "@" + newmember.UserName
	} else {
		username = newmember.FirstName
	}

	notifyMessage := notify(joinMessage.Chat.ID, fmt.Sprintf(
		"Hello, %s. You have 15min to pay the following invoice for %d sat if you want to stay in this group:",
		username, sats))

	ln.Call("delinvoice", label, "unpaid")  // we don't care if it doesn't exist
	ln.Call("delinvoice", label, "paid")    // we don't care if it doesn't exist
	ln.Call("delinvoice", label, "expired") // we don't care if it doesn't exist

	chatOwner, err := getChatOwner(joinMessage.Chat.ID)
	if err != nil {
		log.Warn().Err(err).Msg("chat has no owner, failed to create a ticket invoice. allowing user.")
		return
	}

	expiration := time.Minute * 15

	bolt11, hash, qrpath, err := chatOwner.makeInvoice(sats, fmt.Sprintf(
		"ticket for %s to join %s (%d).",
		username, joinMessage.Chat.Title, joinMessage.Chat.ID,
	), label, &expiration, nil, "", false)

	invoiceMessage := notifyWithPicture(joinMessage.Chat.ID, qrpath, bolt11)

	kickdata := KickData{
		invoiceMessage,
		notifyMessage,
		*joinMessage,
		tgbotapi.ChatMemberConfig{
			UserID: newmember.ID,
			ChatID: joinMessage.Chat.ID,
		},
		newmember,
		hash,
	}

	kickdatajson, _ := json.Marshal(kickdata)
	err = rds.HSet("ticket-pending", label, string(kickdatajson)).Err()
	if err != nil {
		log.Warn().Err(err).Str("kickdata", string(kickdatajson)).Msg("error saving kickdata")
	}
	pendingApproval[label] = kickdata
	go waitToKick(label, kickdata)
}

func waitToKick(label string, kickdata KickData) {
	log.Debug().Str("label", label).Msg("waiting to kick")
	invpaid, err := ln.CallWithCustomTimeout(time.Minute*60, "waitinvoice", label)
	if err == nil && invpaid.Get("status").String() == "paid" {
		// the user did pay. allow.
		ticketPaid(label, kickdata)
		return
	} else if err != nil {
		if cmderr, ok := err.(lightning.ErrorCommand); ok {
			if cmderr.Code == -1 {
				log.Info().Str("label", label).
					Msg("invoice deleted, assume it was paid internally")
				ticketPaid(label, kickdata)
				return
			} else if cmderr.Code == -2 {
				if _, isPending := pendingApproval[label]; !isPending {
					// not pending anymore, means the invoice was paid internally. don't kick.
					return
				}

				// didn't pay. kick.
				log.Info().Str("label", label).Msg("invoice expired, kicking user")

				banuntil := time.Now()
				banuntil.AddDate(0, 0, 1)

				bot.KickChatMember(tgbotapi.KickChatMemberConfig{
					kickdata.ChatMemberConfig,
					banuntil.Unix(),
				})

				delete(pendingApproval, label)
				rds.HDel("ticket-pending", label)

				// delete messages
				deleteMessage(&kickdata.JoinMessage)
				deleteMessage(&kickdata.NotifyMessage)
				deleteMessage(&kickdata.InvoiceMessage)
				return
			}
		}
		log.Warn().Err(err).Msg("unexpected error while waiting to kick")
	} else {
		// should never happen
		log.Error().Str("invoice", invpaid.String()).
			Msg("got a response for an invoice that wasn't paid. shouldn't have happened.")
	}
}

func ticketPaid(label string, kickdata KickData) {
	log.Debug().Str("label", label).Msg("ticket paid")
	delete(pendingApproval, label)
	rds.HDel("ticket-pending", label)

	// delete the invoice message
	deleteMessage(&kickdata.InvoiceMessage)

	user, _, _ := ensureUser(kickdata.NewMember.ID, kickdata.NewMember.UserName)

	// replace caption
	_, err := bot.Send(tgbotapi.NewEditMessageText(
		kickdata.NotifyMessage.Chat.ID,
		kickdata.NotifyMessage.MessageID,
		"Invoice paid. "+user.AtName()+" allowed.",
	))
	if err != nil {
		log.Warn().Err(err).Msg("failed to replace invoice with 'paid' message.")
	}
}

func startKicking() {
	data, err := rds.HGetAll("ticket-pending").Result()
	if err != nil {
		log.Warn().Err(err).Msg("error getting tickets pending")
		return
	}

	for label, kickdatastr := range data {
		var kickdata KickData
		err := json.Unmarshal([]byte(kickdatastr), &kickdata)
		if err != nil {
			log.Warn().Err(err).Msg("failed to unmarshal kickdata from redis")
			continue
		}

		log.Debug().Msg("restarted kick invoice wait")
		pendingApproval[label] = kickdata
		go waitToKick(label, kickdata)
	}
}

func interceptMessage(message *tgbotapi.Message) (proceed bool) {
	label := fmt.Sprintf("newmember:%d:%d", message.From.ID, message.Chat.ID)
	if _, isPending := pendingApproval[label]; isPending {
		log.Debug().Str("user", message.From.String()).Msg("user pending, can't speak")
		return false
	}
	return true
}
