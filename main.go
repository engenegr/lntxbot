package main

import (
	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fiatjaf/lightningd-gjson-rpc"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"gopkg.in/redis.v5"
)

type Settings struct {
	ServiceId   string `envconfig:"SERVICE_ID" default:"lntxbot"`
	ServiceURL  string `envconfig:"SERVICE_URL" required:"true"`
	Port        string `envconfig:"PORT" required:"true"`
	BotToken    string `envconfig:"BOT_TOKEN" required:"true"`
	PostgresURL string `envconfig:"DATABASE_URL" required:"true"`
	RedisURL    string `envconfig:"REDIS_URL" required:"true"`
	SocketPath  string `envconfig:"SOCKET_PATH" required:"true"`

	InvoiceTimeout       time.Duration `envconfig:"INVOICE_TIMEOUT" default:"24h"`
	PayConfirmTimeout    time.Duration `envconfig:"PAY_CONFIRM_TIMEOUT" default:"5h"`
	GiveAwayTimeout      time.Duration `envconfig:"GIVE_AWAY_TIMEOUT" default:"5h"`
	HiddenMessageTimeout time.Duration `envconfig:"HIDDEN_MESSAGE_TIMEOUT" default:5d"`

	NodeId string
	Usage  string
}

var err error
var s Settings
var pg *sqlx.DB
var ln *lightning.Client
var rds *redis.Client
var bot *tgbotapi.BotAPI
var log = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr})
var bundle *i18n.Bundle

func main() {
	err = envconfig.Process("", &s)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't process envconfig.")
	}

	langFiles := []string{"translations/en.toml", "translations/es.toml", "translations/ru.toml"}

	bundle, err = CreateLocalizerBundle(langFiles)
	if err != nil {
		log.Fatal().Err(err).Msg("error initialising localization")
		panic(err)
	}

	setupCommands()

	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log = log.With().Timestamp().Logger()

	// seed the random generator
	rand.Seed(time.Now().UnixNano())

	// postgres connection
	pg, err = sqlx.Connect("postgres", s.PostgresURL)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't connect to postgres")
	}

	// redis connection
	rurl, _ := url.Parse(s.RedisURL)
	pw, _ := rurl.User.Password()
	rds = redis.NewClient(&redis.Options{
		Addr:     rurl.Host,
		Password: pw,
	})
	if err := rds.Ping().Err(); err != nil {
		log.Fatal().Err(err).Str("url", s.RedisURL).
			Msg("failed to connect to redis")
	}

	// create bot
	bot, err = tgbotapi.NewBotAPI(s.BotToken)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	log.Info().Str("username", bot.Self.UserName).Msg("telegram bot authorized")

	// lightningd connection
	lastinvoiceindex, err := rds.Get("lastinvoiceindex").Int64()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get lastinvoiceindex from redis")
		return
	}
	if lastinvoiceindex < 10 {
		res, err := ln.Call("listinvoices")
		if err != nil {
			log.Fatal().Err(err).Msg("failed to get lastinvoiceindex from listinvoices")
			return
		}
		indexes := res.Get("invoices.#.pay_index").Array()
		for _, indexr := range indexes {
			index := indexr.Int()
			if index > lastinvoiceindex {
				lastinvoiceindex = index
			}
		}
	}

	ln = &lightning.Client{
		Path:             s.SocketPath,
		LastInvoiceIndex: int(lastinvoiceindex),
		PaymentHandler:   invoicePaidListener,
	}
	ln.ListenForInvoices()

	// bot stuff
	_, err = bot.SetWebhook(tgbotapi.NewWebhook(s.ServiceURL + "/" + bot.Token))
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	info, err := bot.GetWebhookInfo()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	if info.LastErrorDate != 0 {
		log.Debug().Str("err", info.LastErrorMessage).Msg("telegram callback failed")
	}
	updates := bot.ListenForWebhook("/" + bot.Token)

	// handle QR code requests from telegram
	http.HandleFunc("/qr/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[3:]
		if strings.HasPrefix(path, "/tmp/") && strings.HasSuffix(path, ".png") {
			http.ServeFile(w, r, path)
		} else {
			http.Error(w, "not found", 404)
		}
	})

	// lndhub-compatible routes
	startBlueWallet()

	// start http server
	go http.ListenAndServe("0.0.0.0:"+s.Port, nil)

	// pause here until lightningd works
	s.NodeId = probeLightningd()

	// dispatch kick job for pending users
	startKicking()

	for update := range updates {
		handle(update, bundle)
	}
}

func probeLightningd() string {
	nodeinfo, err := ln.Call("getinfo")
	if err != nil {
		log.Warn().Err(err).Msg("can't talk to lightningd. retrying.")
		time.Sleep(time.Second * 5)
		return probeLightningd()
	}
	log.Info().
		Str("id", nodeinfo.Get("id").String()).
		Str("alias", nodeinfo.Get("alias").String()).
		Int64("channels", nodeinfo.Get("num_active_channels").Int()).
		Int64("blockheight", nodeinfo.Get("blockheight").Int()).
		Str("version", nodeinfo.Get("version").String()).
		Msg("lightning node connected")

	return nodeinfo.Get("id").String()
}

// CreateLocalizerBundle reads language files and registers them in i18n bundle
func CreateLocalizerBundle(langFiles []string) (*i18n.Bundle, error) {
	// Bundle stores a set of messages
	bundle = i18n.NewBundle(language.English)

	// Enable bundle to understand yaml
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	var translations []byte
	var err error
	for _, file := range langFiles {
		// Read our language toml file
		translations, err = ioutil.ReadFile(file)
		if err != nil {
			log.Fatal().Err(err).Msg("Unable to read translation file")
			return nil, err
		}
		// It parses the bytes in buffer to add translations to the bundle
		bundle.MustParseMessageFileBytes(translations, file)
	}

	return bundle, nil
}