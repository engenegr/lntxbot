package main

import (
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/kballard/go-shellquote"
)

type def struct {
	explanation    string
	argstr         string
	flags          []flag
	examples       []example
	aliases        []string
	inline         bool
	inline_example string
}

type example struct {
	Value       string
	Explanation string
}

type flag struct {
	Name        string
	Explanation string
}

var methods = []def{
	def{
		aliases: []string{"start"},
	},
	def{
		aliases:     []string{"receive", "invoice", "fund"},
		explanation: "Generates a BOLT11 invoice with given satoshi value. Amounts will be added to your bot balance. If you don't provide the amount it will be an open-ended invoice that can be paid with any amount.",
		// the "any" is here only for illustrative purposes. if you call this with 'any' it will
		// actually be assigned to the <satoshis> variable, and that's how the code handles it.
		argstr: "(<satoshis>|any) [<description>...] [--preimage=<preimage>]",
		flags: []flag{
			{
				"--preimage",
				"If you want to generate an invoice with a specific preimage, write it here as a 32b / 64char hex string.",
			},
		},
		examples: []example{
			{
				"/receive 320 for something",
				"Generates an invoice for 320 sat with the description \"for something\".",
			},
			{
				"/invoice any",
				"Generates an invoice with undefined amount.",
			},
		},
		inline:         true,
		inline_example: "invoice &lt;satoshis&gt;",
	},
	def{
		aliases:     []string{"pay", "decode", "paynow", "withdraw"},
		explanation: "Decodes a BOLT11 invoice and asks if you want to pay it (unless `/paynow`). This is the same as just pasting or forwarding an invoice directly in the chat. Taking a picture of QR code containing an invoice works just as well (if the picture is clear).",
		argstr:      "[now] [<invoice>] [<satoshis>]",
		examples: []example{
			{
				"/pay lnbc1u1pwvmypepp5kjydaerr6rawl9zt7t2zzl9q0rf6rkpx7splhjlfnjr869we3gfqdq6gpkxuarcvfhhggr90psk6urvv5cqp2rzjqtqkejjy2c44jrwj08y5ygqtmn8af7vscwnflttzpsgw7tuz9r407zyusgqq44sqqqqqqqqqqqqqqqgqpcxuncdelh5mtthgwmkrum2u5m6n3fcjkw6vdnffzh85hpr4tem3k3u0mq3k5l3hpy32ls2pkqakpkuv5z7yms2jhdestzn8k3hlr437cpajsnqm",
				"Asks if you want to pay this invoice for 100 sat.",
			},
			{
				"/paynow lnbc1u1pwvmypepp5kjydaerr6rawl9zt7t2zzl9q0rf6rkpx7splhjlfnjr869we3gfqdq6gpkxuarcvfhhggr90psk6urvv5cqp2rzjqtqkejjy2c44jrwj08y5ygqtmn8af7vscwnflttzpsgw7tuz9r407zyusgqq44sqqqqqqqqqqqqqqqgqpcxuncdelh5mtthgwmkrum2u5m6n3fcjkw6vdnffzh85hpr4tem3k3u0mq3k5l3hpy32ls2pkqakpkuv5z7yms2jhdestzn8k3hlr437cpajsnqm",
				"Pays this invoice without asking for confirmation.",
			},
			{
				"/pay lnbc1pwvm0pxpp5n2qa3pnfmu7p9vaqspn2cwp7ej44mh6tf77pnxpvfked8z5wg64sdqlypdkcmn50p3x7ap0gpnxjct5dfskvhgxqyz5vqcqp2rzjqfxj8p6qjf5l8du7yuytkwdcjhylfd4gxgs48t65awjg04ye80mq7zyhg5qq5ysqqqqqqqqqqqqqqqsqrcaycpuwzwv4u5yg94ne4ct2lrkmleuq4ly5qcjueuu6qkx5d4qdun5xx0wxp6djch093svm06szy0ru9kvcpmzs7vzjpvxfwyep8fugsq96d3ww 3000",
				"Asks if you want to pay 3000 sat for this invoice with undefined amount.",
			},
			{
				"/pay",
				"When sent as a reply to another message containing an invoice (for example, in a group), asks privately if you want to pay it.",
			},
		},
	},
	def{
		aliases:     []string{"send", "tip", "sendanonymously"},
		explanation: "Sends satoshis to other Telegram users. The receiver is notified on his chat with the bot. If the receiver has never talked to the bot or have blocked it he can't be notified, however. In that case you can cancel the transaction afterwards in the /transactions view.",
		argstr:      "[anonymously] <satoshis> [<receiver>...] [--anonymous]",
		flags: []flag{
			{
				"--anonymous",
				"The receiver will never know who sent him the satoshis.",
			},
		},
		examples: []example{
			{
				"/send 500 @username",
				"Sends 500 satoshis to Telegram user @username.",
			},
			{
				"/tip 100",
				"When sent as a reply to a message in a group where the bot is added, this will send 100 satoshis to the author of the message.",
			},
			{
				"/send anonymously 1000 @someone",
				"Telegram user @someone will see just: \"Someone has sent you 1000 satoshis\".",
			},
		},
	},
	def{
		aliases:     []string{"balance"},
		explanation: "Shows your current balance in satoshis, plus the sum of everything you've received and sent within the bot and the total amount of fees paid.",
	},
	def{
		aliases:     []string{"transactions"},
		explanation: "Lists your recent transactions, including internal and external payments, giveaways, tips, coinflips and everything else. Each transaction will have a unique identifier in the form of /tx___ that you can click for more info and extra actions, when available.",
		flags: []flag{
			{
				"--page",
				"To show older transactions, specify a page number greater than 1.",
			},
		},
		argstr: "[--page=<page>]",
	},
	def{
		aliases:     []string{"giveaway"},
		explanation: "Creates a button in a group chat. The first person to click the button gets the satoshis.",
		argstr:      "<satoshis>",
		examples: []example{
			{
				"/giveaway 1000",
				"Once someone clicks the \"Claim\" button 1000 satoshis will be transferred from you to them.",
			},
		},
		inline:         true,
		inline_example: "giveaway &lt;satoshis&gt;",
	},
	def{
		aliases:     []string{"coinflip", "lottery"},
		explanation: "Starts a fair lottery with the given number of participants. Everybody pay the same amount as the entry fee. The winner gets it all. Funds are only moved from participants accounts when the lottery is actualized.",
		argstr:      "<satoshis> [<num_participants>]",
		examples: []example{
			{
				"/coinflip 100 5",
				"5 participants needed, winner will get 500 satoshis (including its own 100, so it's 400 net satoshis).",
			},
		},
		inline:         true,
		inline_example: "coinflip &lt;satoshis&gt; &lt;num_participants&gt;",
	},
	def{
		aliases:     []string{"giveflip"},
		explanation: "Starts a giveaway, but instead of giving to the first person who clicks, the amount is raffled between first x clickers.",
		argstr:      "<satoshis> <num_participants>",
		examples: []example{
			{
				"/giveflip 100 5",
				"5 participants needed, winner will get 500 satoshis from the command issuer.",
			},
		},
		inline:         true,
		inline_example: "giveflip &lt;satoshis&gt; &lt;num_participants&gt;",
	},
	def{
		aliases:     []string{"fundraise", "crowdfund"},
		explanation: "Starts a crowdfunding event with a predefined number of participants and contribution amount. If the given number of participants contribute, it will be actualized. Otherwise it will be canceled in some hours.",
		argstr:      "<satoshis> <num_participants> <receiver>...",
		examples: []example{
			{
				"/fundraise 10000 8 @user",
				"Telegram @user will get 80000 satoshis after 8 people contribute.",
			},
		},
	},
	def{
		aliases:     []string{"bluewallet", "lndhub"},
		explanation: "Returns your credentials for importing your bot wallet on BlueWallet. You can use the same account from both places interchangeably.",
		argstr:      "[refresh]",
		examples: []example{
			{
				"/bluewallet",
				"Prints a string like `lndhub://<login>:<password>@<url>` which must be copied and pasted on BlueWallet's import screen.",
			},
			{
				"/bluewallet refresh",
				"Erases your previous password and prints a new string. You'll have to reimport the credentials on BlueWallet after this step. Only do it if your previous credentials were compromised.",
			},
		},
	},
	def{
		aliases:     []string{"toggle"},
		explanation: "Toggles bot features in groups on/off. In supergroups it only be run by group admins.",
		argstr:      "(ticket [<price>]|spammy)",
		examples: []example{
			{
				"/toggle ticket 10",
				"New group entrants will be prompted to pay 10 satoshis in 30 minutes or be kicked. Useful as an antispam measure.",
			},
			{
				"/toggle ticket",
				"Stop charging new entrants a fee.",
			},
			{
				"/toggle spammy",
				"'spammy' mode is off by default. When turned on, tip notifications will be sent in the group instead of only privately.",
			},
		},
	},
	def{
		aliases:     []string{"help"},
		explanation: "Shows full help or help about specific command.",
		argstr:      "[<command>]",
	},
	def{
		aliases:     []string{"stop"},
		explanation: "The bot stops showing you notifications.",
	},
}

var commandList []string
var commandIndex = make(map[string]def)

func setupCommands() {
	s.Usage = docoptFromMethodDefinitions()

	for _, def := range methods {
		for _, alias := range def.aliases {
			commandList = append(commandList, alias)
			commandIndex[alias] = def
		}
	}
}

func docoptFromMethodDefinitions() string {
	var lines []string

	for _, def := range methods {
		for _, alias := range def.aliases {
			lines = append(lines, "  c "+alias+" "+def.argstr)
		}
	}

	return s.ServiceId + "\n\nUsage:\n" + strings.Join(lines, "\n")
}

var parser = docopt.Parser{
	HelpHandler:   func(_ error, _ string) {},
	OptionsFirst:  false,
	SkipHelpFlags: true,
}

func parse(message string) (opts docopt.Opts, isCommand bool, err error) {
	if strings.HasPrefix(message, "/") {
		isCommand = true
		message = message[1:]
	} else {
		return
	}

	var argv []string
	argv, err = shellquote.Split(message)
	if err != nil {
		return
	}

	opts, err = parser.ParseArgs(s.Usage, argv, "")
	return
}

func handleHelp(u User, method string, locale string) (handled bool) {
	var def def
	var mainName string
	var aliases []map[string]string
	var helpString string
	var ok bool
	var argHelpKey, descHelpKey, examHelpKey, headerStr, argsStr, descStr, examStr, aliasesStr, inlineStr string
	method = strings.ToLower(strings.TrimSpace(method))
	msgTempl := map[string]interface{}{}
	if method == "" {
		msgTempl = map[string]interface{}{
			"Help": escapeHTML(strings.Replace(s.Usage, "  c ", "  /", -1)),
		}
		msgStr0, _ := translateTemplate("HelpIntro", locale, msgTempl)
		msgStr1, _ := translateTemplate("HelpString", locale, msgTempl)
		helpString = msgStr0 + "\n" + msgStr1
		goto gothelpstring
	}

	// render specific help instructions for the given method
	def, ok = commandIndex[method]
	if ok {
		mainName = method
		aliases = make([]map[string]string, len(def.aliases)-1)
		i := 0
		for _, alias := range def.aliases {
			if alias != mainName {
				aliases[i] = map[string]string{"alias": alias}
				i++
			}
		}
	} else {
		similar := findSimilar(method, commandList)
		if len(similar) > 0 {
			msgTempl := map[string]interface{}{
				"Method": method,
				"Similar": similar[0],
			}
			msgStr, _ := translateTemplate("SimilarMsg", locale, msgTempl)
			reply := msgStr
			if len(similar) > 1 {
				msgTempl = map[string]interface{}{
					"Similar": similar[1],
				}
				msgStr, _ = translateTemplate("SimilarOrMaybe", locale, msgTempl)
				reply += msgStr
				if len(similar) > 2 {
					msgTempl = map[string]interface{}{
						"Similar": similar[2],
					}
					msgStr, _ = translateTemplate("SimilarPerhaps", locale, msgTempl)
					reply += msgStr
				}
			}
			u.notify(reply)
			return true
		} else {
			return false
		}
	}

	// here we have a working method definition
	argHelpKey = mainName+"HelpArgs"
	argsStr, _ = translate(argHelpKey, locale)
	descHelpKey = mainName+"HelpDesc"
	descStr, _ = translate(descHelpKey, locale)
	examHelpKey = mainName+"HelpExam"
	examStr, _ = translate(examHelpKey, locale)

	if examStr == "" {
		examStr, _ = translate("No", locale)
	}

	if def.inline {
		inlineTempl := map[string]interface{}{
			"ServiceId": s.ServiceId,
			"InlineExample": def.inline_example,
		}
		inlineStr, _ = translateTemplate("InlineHelp", locale, inlineTempl)
	} else {
		inlineStr, _ = translate("NotSupported", locale)
	}

	if len(aliases) > 0 {
		aliasesStr = ""
		for _, alias := range def.aliases {
			aliasesStr += alias+" "
		}
	} else {
		aliasesStr, _ = translate("NotSupported", locale)
	}

	msgTempl = map[string]interface{}{
		"MainName": mainName,
		"Args": argsStr,
		"Desc": descStr,
		"Exam": examStr,
		"Inline": inlineStr,
		"Aliases": aliasesStr,
	}

	headerStr, _ = translateTemplate("MethodHelpHeader", locale, msgTempl)
	helpString = headerStr

	goto gothelpstring

gothelpstring:
	u.notify(helpString)
	return true
}