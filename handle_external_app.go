package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/go-telegram-bot-api/telegram-bot-api"
)

func handleExternalApp(u User, opts docopt.Opts, messageId int) {
	switch {
	case opts["microbet"].(bool):
		if opts["bet"].(bool) {
			// list available bets as actionable buttons
			bets, err := getMicrobetBets()
			if err != nil {
				u.notify(err.Error())
				return
			}

			inlinekeyboard := make([][]tgbotapi.InlineKeyboardButton, 2*len(bets))
			for i, bet := range bets {
				parts := strings.Split(bet.Description, "→")
				gamename := parts[0]
				backbet := parts[1]
				if bet.Exact {
					backbet += " (exact)"
				}

				inlinekeyboard[i*2] = []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonURL(
						fmt.Sprintf("%s (%d sat)", gamename, bet.Amount),
						"https://www.google.com/search?q="+gamename,
					),
				}
				inlinekeyboard[i*2+1] = []tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData(
						fmt.Sprintf("%s (%d)", backbet, bet.Backers),
						fmt.Sprintf("app=microbet-%s-true", bet.Id),
					),
					tgbotapi.NewInlineKeyboardButtonData(
						fmt.Sprintf("NOT (%d)", bet.TotalUsers-bet.Backers),
						fmt.Sprintf("app=microbet-%s-false", bet.Id),
					),
				}
			}

			chattable := tgbotapi.NewMessage(u.ChatId, "<b>[Microbet]</b> Bet on one of these predictions:")
			chattable.ParseMode = "HTML"
			chattable.BaseChat.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{inlinekeyboard}
			bot.Send(chattable)
		} else if opts["bets"].(bool) {
			// list my bets
			bets, err := getMyMicrobetBets(u)
			if err != nil {
				u.notify(err.Error())
				return
			}

			if len(bets) == 0 {
				u.notify("<b>[Microbet]</b>: no bets.")
				return
			}

			message := make([]string, 0, len(bets))
			for _, bet := range bets {
				result := "open"
				if bet.Canceled {
					result = "canceled"
				} else if bet.Closed {
					if bet.WonAmount != 0 {
						result = fmt.Sprintf("won %d", bet.WonAmount)
					} else {
						result = "lost"
					}
				}

				if bet.UserBack > 0 {
					message = append(message, fmt.Sprintf("<code>%s</code> <code>%d</code> 🔵 %d/%d × %d ~ <i>%s</i>",
						bet.Description,
						bet.Amount,
						bet.UserBack,
						bet.Backers,
						bet.TotalUsers-bet.Backers,
						result,
					))
				}

				if bet.UserLay > 0 {
					message = append(message, fmt.Sprintf("<code>%s</code> <code>%d</code> 🔴 %d/%d × %d ~ <i>%s</i>",
						bet.Description,
						bet.Amount,
						bet.UserLay,
						bet.TotalUsers-bet.Backers,
						bet.Backers,
						result,
					))
				}
			}

			u.notify("<b>[Microbet]</b> Your bets\n" + strings.Join(message, "\n"))
		} else if opts["balance"].(bool) {
			balance, err := getMicrobetBalance(u)
			if err != nil {
				u.notify("Error fetching Microbet balance: " + err.Error())
				return
			}

			chattable := tgbotapi.NewMessage(u.ChatId,
				fmt.Sprintf(`<b>[Microbet]</b> balance: <i>%d sat</i>`, balance))
			chattable.ParseMode = "HTML"
			chattable.BaseChat.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Withdraw?", "app=microbet-withdraw"),
				),
			)
			bot.Send(chattable)
		} else if opts["withdraw"].(bool) {
			balance, err := getMicrobetBalance(u)
			if err != nil {
				u.notify("Error fetching Microbet balance: " + err.Error())
				return
			}

			err = withdrawMicrobet(u, int(float64(balance)*0.99))
			if err != nil {
				u.notify("Withdraw error: " + err.Error())
				return
			}
		} else {
			u.notify(`
<a href="https://microbet.fun/">Microbet</a> is a simple service that allows people to bet against each other on sports games results. The bet price is fixed and the odds are calculated considering the amount of back versus lay bets. There's a 1% fee on all withdraws.

<b>Commands:</b>

<code>/app microbet bet</code> to list all open bets and then place yours.
<code>/app microbet bets</code> to see all your past bets.
<code>/app microbet balance</code> to view your balance.
<code>/app microbet withdraw</code> to withdraw all your balance.
            `)
		}
	case opts["bitflash"].(bool):
		if opts["orders"].(bool) {
			var data struct {
				Orders []string `json:"orders"`
			}
			err := u.getAppData("bitflash", &data)
			if err != nil {
				u.notify(err.Error())
				return
			}

			if len(data.Orders) == 0 {
				u.notify("<b>[bitflash]</b>: no past orders.")
				return
			}

			message := make([]string, len(data.Orders))
			for i, id := range data.Orders {
				order, err := getBitflashOrder(id)
				if err != nil {
					log.Warn().Err(err).Str("id", id).Msg("error getting bitflash order on list")
					continue
				}

				amount := strings.Split(strings.Split(order.Description, " of ")[1], " to ")[0]
				address := strings.Split(strings.Split(order.Description, " to ")[1], "(")[0]
				status := fmt.Sprintf("pending since %s", time.Unix(order.CreatedAt, 0).Format("2 Jan 15:04"))
				if order.PaidAt > 0 {
					status = fmt.Sprintf("queued at %s", time.Unix(order.PaidAt, 0).Format("2 Jan 15:04"))
				}

				message[i] = fmt.Sprintf(
					`🧱 <code>%s</code> to <code>%s</code> <i>%s</i>`,
					amount, address, status,
				)
			}

			u.notify("<b>[bitflash]</b> Your past orders\n" + strings.Join(message, "\n"))
		} else if opts["status"].(bool) {

		} else if opts["rate"].(bool) {

		} else {
			// queue a transaction or show help if no arguments
			satoshis, err1 := opts.Int("<satoshis>")
			address, err2 := opts.String("<address>")

			if err1 != nil || err2 != nil {
				u.notify(`
<a href="https://bitflash.club/">Bitflash</a> is a service that does cheap onchain transactions from Lightning payments. It does it cheaply because it aggregates many Lightning transactions and then dispatches them to the chain after a certain threshold is reached.

<b>Commands:</b>

<code>/app bitflash &lt;satoshi_amount&gt; &lt;bitcoin_address&gt;</code> to queue a transaction.
<code>/app bitflash orders</code> lists your previous transactions.
            `)
				return
			}

			ordercreated, err := prepareBitflashTransaction(u, messageId, satoshis, address)
			if err != nil {
				u.notifyAsReply(err.Error(), messageId)
				return
			}

			inv, _ := ln.Call("decodepay", ordercreated.Bolt11)

			// confirm
			chattable := tgbotapi.NewMessage(u.ChatId, fmt.Sprintf(`<b>[bitflash]</b> Do you confirm you want to queue a Bitflash transaction that will send <b>%s</b> to <code>%s</code>? You will pay <b>%.0f</b>.`, ordercreated.ReceiverAmount, ordercreated.Receiver, inv.Get("msatoshi").Float()/1000))
			chattable.ParseMode = "HTML"
			chattable.BaseChat.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Cancel", fmt.Sprintf("cancel=%d", u.Id)),
					tgbotapi.NewInlineKeyboardButtonData(
						"Confirm!",
						fmt.Sprintf("app=bitflash-%s", ordercreated.ChargeId),
					),
				),
			)
			bot.Send(chattable)
		}
	case opts["satellite"].(bool):
		if opts["transmissions"].(bool) {
			var satdata SatelliteData
			err := u.getAppData("satellite", &satdata)
			if err != nil {
				u.notify("Failed to get stored satellite data: " + err.Error())
				return
			}

			message := make([]string, len(satdata.Orders))
			for i, tuple := range satdata.Orders {
				order, err := fetchSatelliteOrder(tuple[0], tuple[1])
				if err == nil {
					message[i] = formatSatelliteOrderLine(order)
				}
			}

			u.notify("<b>[Satellite]</b> Your transmissions\n" + strings.Join(message, "\n"))
		} else if opts["queue"].(bool) {
			queue, err := getSatelliteQueue()
			if err != nil {
				u.notifyAsReply("Error fetching the queue: "+err.Error(), messageId)
				return
			}

			if len(queue) == 0 {
				u.notify("<b>[Satellite]</b>: no queued transmissions.")
				return
			}

			message := make([]string, len(queue))
			for i, order := range queue {
				message[i] = formatSatelliteOrderLine(order)
			}

			u.notify("<b>[Satellite]</b> Queued transmissions\n" + strings.Join(message, "\n"))
		} else if opts["bump"].(bool) {
			err := bumpSatelliteOrder(u, messageId, opts["<transaction_id>"].(string), opts["<satoshis>"].(int))
			if err != nil {
				u.notifyAsReply("Error bumping transmission: "+err.Error(), messageId)
				return
			}
		} else if opts["delete"].(bool) {
			err := deleteSatelliteOrder(u, opts["<transaction_id>"].(string))
			if err != nil {
				u.notifyAsReply("Error deleting transmission: "+err.Error(), messageId)
				return
			}
			u.notifyAsReply("Transmission deleted.", messageId)
			return
		} else {
			// either show help or create an order
			satoshis, err := opts.Int("<satoshis>")

			if err != nil {
				u.notify(`
The <a href="https://blockstream.com/satellite/">Blockstream Satellite</a> is a service that broadcasts Bitcoin blocks and other transmissions to the entire planet. You can transmit any message you want and pay with some satoshis.

<b>Commands:</b>

<code>/app satellite &lt;bid_satoshis&gt; &lt;message...&gt;</code> to queue a transmission.
<code>/app satellite transmissions</code> lists your transmissions.
<code>/app satellite queue</code> lists the next queued transmissions.
<code>/app satellite bump &lt;bid_increase_satoshis&gt; &lt;message_id&gt;</code> to increaase the bid for a transmission.
<code>/app satellite delete &lt;message_id&gt;</code> to delete a transmission.
            `)
				return
			}

			// create an order
			var message string
			if imessage, ok := opts["<message>"]; ok {
				message = strings.Join(imessage.([]string), " ")
			}

			err = createSatelliteOrder(u, messageId, satoshis, message)
			if err != nil {
				u.notifyAsReply("Error making transmission: "+err.Error(), messageId)
				return
			}
		}
	}
}

func handleExternalAppCallback(u User, messageId int, cb *tgbotapi.CallbackQuery) (answer string) {
	parts := strings.Split(cb.Data[4:], "-")
	switch parts[0] {
	case "microbet":
		if parts[1] == "withdraw" {
			balance, err := getMicrobetBalance(u)
			if err != nil {
				u.notify("Error fetching Microbet balance: " + err.Error())
				return "Failure."
			}

			err = withdrawMicrobet(u, int(float64(balance)*0.99))
			if err != nil {
				u.notify("Withdraw error: " + err.Error())
				return "Failure."
			}

			return "Withdrawing."
		} else {
			betId := parts[1]
			back := parts[2] == "true"
			bet, err := getMicrobetBet(betId)
			if err != nil {
				return "Bet not available."
			}

			// post a notification message to identify this bet attempt
			gamename := strings.Split(bet.Description, "→")[0]
			message := u.notify(fmt.Sprintf("Placing bet on <b>%s</b>.", strings.TrimSpace(gamename)))

			err = placeMicrobetBet(u, message.MessageID, betId, back)
			if err != nil {
				u.notify(err.Error())
				return "Failure."
			}

			return "Placing bet."
		}
	case "bitflash":
		chargeId := parts[1]

		// get data for this charge
		order, err := getBitflashOrder(chargeId)
		if err != nil {
			u.notify(err.Error())
			return "Failure."
		}

		// pay it - just paying the invoice is enough
		err = payBitflashInvoice(u, order, messageId)
		if err != nil {
			u.notify(err.Error())
			return "Failure."
		}

		// store order id so we can show it later on /app bitflash orders
		saveBitflashOrder(u, order.Id)

		removeKeyboardButtons(cb)
		return "Queueing Bitflash transaction."
	}

	return
}
