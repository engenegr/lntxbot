package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func startLndHub() {
	http.HandleFunc("/getinfo", func(w http.ResponseWriter, r *http.Request) {
		log.Debug().Msg("lndhub /getinfo")
		errorBadAuth(w)
	})

	http.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		var params struct {
			Login    string `json:"login"`
			Password string `json:"password"`
		}
		err := json.NewDecoder(r.Body).Decode(&params)
		if err != nil {
			errorInvalidParams(w)
			return
		}
		log.Debug().Str("login", params.Login).Str("password", params.Password[:5]).Msg("lndhub /auth")

		token := base64.StdEncoding.EncodeToString([]byte(params.Login + ":" + params.Password))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			RefreshToken string `json:"refresh_token"`
			AccessToken  string `json:"access_token"`
		}{token, token})
	})

	http.HandleFunc("/addinvoice", func(w http.ResponseWriter, r *http.Request) {
		user, err := loadUserFromBlueWalletCall(r)
		if err != nil {
			errorBadAuth(w)
			return
		}

		var params struct {
			Amount string `json:"amt"`
			Memo   string `json:"memo"`
		}
		err = json.NewDecoder(r.Body).Decode(&params)
		if err != nil {
			errorInvalidParams(w)
			return
		}

		msatoshi, err := strconv.Atoi(params.Amount)
		if err != nil {
			errorInvalidParams(w)
			return
		}

		bolt11, hash, _, err := user.makeInvoice(msatoshi, params.Memo, "", nil, nil, "")
		if err != nil {
			errorInternal(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			PayReq         string `json:"pay_req"`
			PaymentRequest string `json:"payment_request"`
			AddIndex       string `json:"add_index"`
			RHash          Buffer `r_hash`
		}{bolt11, bolt11, "1000", Buffer(hash)})
	})

	http.HandleFunc("/payinvoice", func(w http.ResponseWriter, r *http.Request) {
		user, err := loadUserFromBlueWalletCall(r)
		if err != nil {
			errorBadAuth(w)
			return
		}

		var params struct {
			Invoice string `json:"invoice"`
		}
		err = json.NewDecoder(r.Body).Decode(&params)
		if err != nil {
			errorInvalidParams(w)
			return
		}

		log.Debug().Str("bolt11", params.Invoice).Msg("lndhub /payinvoice")

		err = user.payInvoice(0, params.Invoice, 0)
		if err != nil {
			errorPaymentFailed(w, err)
			return
		}

		decoded, _ := decodeInvoiceAsLndHub(params.Invoice)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			PaymentError    string                 `json:"payment_error"`
			PaymentPreimage Buffer                 `json:"payment_preimage"`
			PaymentRoute    map[string]interface{} `json:"route"`
			PaymentHash     Buffer                 `json:"payment_hash"`
			Decoded         Decoded                `json:"decoded"`
		}{"", "", make(map[string]interface{}), "", decoded})
	})

	http.HandleFunc("/balance", func(w http.ResponseWriter, r *http.Request) {
		user, err := loadUserFromBlueWalletCall(r)
		if err != nil {
			errorBadAuth(w)
			return
		}

		info, err := user.getInfo()
		if err != nil {
			errorInternal(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]map[string]float64{
			"BTC": {
				"AvailableBalance": info.Balance / 1000,
			},
		})
	})

	http.HandleFunc("/gettxs", func(w http.ResponseWriter, r *http.Request) {
		user, err := loadUserFromBlueWalletCall(r)
		if err != nil {
			errorBadAuth(w)
			return
		}

		txns, err := user.listTransactions(100, 0)
		if err != nil {
			errorInternal(w)
			return
		}

		type Payment struct {
			PaymentPreimage string  `json:"payment_preimage"`
			Type            string  `json:"type"`
			Fee             float64 `json:"fee"`
			Value           float64 `json:"value"`
			Timestamp       int64   `json:"timestamp"`
			Memo            string  `json:"memo"`
		}

		var payments []Payment
		for _, txn := range txns {
			if txn.Amount > 0 {
				continue
			}

			payments = append(payments, Payment{
				txn.Preimage.String,
				"paid_invoice",
				txn.Fees,
				-txn.Amount,
				txn.Time.Unix(),
				txn.Description,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payments)
	})

	http.HandleFunc("/getpending", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
	})

	http.HandleFunc("/getuserinvoices", func(w http.ResponseWriter, r *http.Request) {
		user, err := loadUserFromBlueWalletCall(r)
		if err != nil {
			errorBadAuth(w)
			return
		}

		txns, err := user.listTransactions(100, 0)
		if err != nil {
			errorInternal(w)
			return
		}

		type Inv struct {
			RHash          Buffer  `json:"r_hash"`
			PaymentRequest string  `json:"payment_request"`
			PayReq         string  `json:"pay_req"`
			AddIndex       string  `json:"add_index"`
			Description    string  `json:"description"`
			PaymentHash    string  `json:"payment_hash"`
			IsPaid         bool    `json:"ispaid"`
			Amount         float64 `json:"amt"`
			ExpireTime     int64   `json:"expire_time"`
			Timestamp      int64   `json:"timestamp"`
			Type           string  `json:"type"`
		}

		var invs []Inv
		for _, txn := range txns {
			if txn.Amount < 0 {
				continue
			}

			invs = append(invs, Inv{
				Buffer(txn.Hash),
				"",
				"",
				"1000",
				txn.Description,
				txn.Hash,
				true,
				txn.Amount,
				int64(s.InvoiceTimeout.Seconds()),
				txn.Time.Unix(),
				"user_invoice",
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(invs)
	})

	http.HandleFunc("/decodeinvoice", func(w http.ResponseWriter, r *http.Request) {
		bolt11 := r.URL.Query().Get("invoice")

		decoded, err := decodeInvoiceAsLndHub(bolt11)
		if err != nil {
			errorInternal(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(decoded)
	})
}

func loadUserFromBlueWalletCall(r *http.Request) (user User, err error) {
	token := strings.Split(strings.TrimSpace(r.Header.Get("Authorization")), " ")[1]
	res, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return
	}

	parts := strings.Split(string(res), ":")
	userId, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	password := parts[1]

	// check password
	if password != userPassword(userId) {
		err = errors.New("invalid password")
		return
	}

	user, err = loadUser(userId, 0)
	return
}

func userPassword(userId int) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s.BotToken+"?"+strconv.Itoa(userId))))
}

type Buffer string

func (b Buffer) MarshalJSON() ([]byte, error) {
	arrayBytes, err := hex.DecodeString(string(b))
	if err != nil {
		return nil, err
	}
	arr := make([]int, len(arrayBytes))
	for i, b := range arrayBytes {
		arr[i] = int(b)
	}
	return json.Marshal(map[string]interface{}{
		"type": "Buffer",
		"data": arr,
	})
}

type Decoded struct {
	Destination     string      `json:"destination"`
	PaymentHash     string      `json:"payment_hash"`
	NumSatoshis     string      `json:"num_satoshis"`
	Timestamp       string      `json:"timestamp"`
	Expiry          string      `json:"expiry"`
	Description     string      `json:"description"`
	DescriptionHash string      `json:"description_hash"`
	FallbackAddr    string      `json:"fallback_addr"`
	CLTVExpiry      string      `json:"cltv_expiry"`
	RouteHints      interface{} `json:"route_hints"`
}

func decodeInvoiceAsLndHub(bolt11 string) (Decoded, error) {
	inv, err := ln.Call("decodepay", bolt11)
	if err != nil {
		return Decoded{}, err
	}

	return Decoded{
		Destination:     inv.Get("payee").String(),
		PaymentHash:     inv.Get("payment_hash").String(),
		NumSatoshis:     strconv.Itoa(int(inv.Get("msatoshi").Float() / 1000.0)),
		Timestamp:       inv.Get("created_at").String(),
		Expiry:          inv.Get("expiry").String(),
		Description:     inv.Get("description").String(),
		DescriptionHash: inv.Get("description_hash").String(),
		FallbackAddr:    inv.Get("fallbacks.0.addr").String(),
		CLTVExpiry:      inv.Get("min_final_cltv_expiry").String(),
		RouteHints:      inv.Get("routes").Value(),
	}, nil
}

func errorInvalidParams(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
      "error": true,
      "code": 8,
      "message": "invalid params"
    }`))
}

func errorBadAuth(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
      "error": true,
      "code": 1,
      "message": "bad auth"
    }`))
}

func errorPaymentFailed(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
      "error": true,
      "code": 10,
      "message": "` + err.Error() + `"
    }`))
}

func errorInternal(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
      "error": true,
      "code": 7,
      "message": "Internal failure"
    }`))
}
