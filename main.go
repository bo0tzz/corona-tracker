package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	lastHash, channelName string
	recentRecords         []*Record
	bot                   *tgbotapi.BotAPI
)

func poll() {
	resp, err := http.Get("https://docs.google.com/spreadsheets/d/1yZv9w9zRKwrGTaR-YzmAqMefw4wMlaXocejdxZaTs6w/export?format=csv")

	if err != nil {
		panic(err)
	}

	d, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		panic(err)
	}

	newHash := fmt.Sprintf("%02x", sha256.Sum256(d))

	if newHash != lastHash {
		newRecords := ReadRecords(bytes.NewReader(d))
		lastHash = newHash
		totalDeaths, totalCases, totalRecover := 0, 0, 0
		totalDeathDiff, totalCasesDiff, totalRecoveredDiff := 0, 0, 0
		for _, record := range newRecords {
			totalCases += record.ConfirmedCases
			totalDeaths += record.Deaths
			totalRecover += record.Recovered
		}

		// analyze diff
		if len(recentRecords) != 0 {
			diffs := DiffRecords(recentRecords, newRecords)

			diffCount := 0
			for idx, diff := range diffs {
				diffCount++
				diffReport := ""
				if diff.Added {
					diffReport += "⚠ *New Outbreak* ⚠\n"
				}

				record := newRecords[idx]

				// ⬆
				// i don't really like this code ngl
				if diff.DeltaCases != 0 || diff.DeltaDeaths != 0 || diff.DeltaRecovered != 0 || diff.Added {

					location := record.Country
					if record.Province != "" {
						location = record.Province + ", " + record.Country
					}
					diffReport += fmt.Sprintf("Update for %s\n", location)

					diffReport += fmt.Sprintf(" Cases: %d ", record.ConfirmedCases)

					if diff.DeltaCases != 0 {
						diffReport += fmt.Sprintf("(+%d)", diff.DeltaCases)
						totalCasesDiff += diff.DeltaCases
					}
					diffReport += ","
					diffReport += fmt.Sprintf("Deaths: %d", record.Deaths)
					if diff.DeltaDeaths != 0 {
						diffReport += fmt.Sprintf("(+%d)", diff.DeltaDeaths)
						totalDeathDiff += diff.DeltaDeaths
					}

					diffReport += ","
					if diff.DeltaRecovered != 0 {
						diffReport += fmt.Sprintf("(+%d)", diff.DeltaRecovered)
						totalRecoveredDiff += diff.DeltaRecovered
					}
					diffReport += "\n\n"

					msg := tgbotapi.MessageConfig{
						BaseChat: tgbotapi.BaseChat{
							ChannelUsername: fmt.Sprintf("@%s", channelName),
						},
						Text:      diffReport,
						ParseMode: "markdownv2",
					}

					if _, err := bot.Send(msg); err != nil {
						recover()
					}
				}
			}
		}

		if bot != nil {
			msg := tgbotapi.MessageConfig{
				BaseChat: tgbotapi.BaseChat{
					ChannelUsername: fmt.Sprintf("@%s", channelName),
				},
				Text: fmt.Sprintf("❗*Coronavirus Updates*❗\n\n*Total Cases: %d \\(\\+%d\\)*\n*Total Deaths: %d \\(\\+%d\\)*\n*Total Recovered: %d \\(\\+%d\\)*\n",
					totalCases, totalCasesDiff, totalDeaths, totalDeathDiff, totalRecover, totalRecoveredDiff),
				ParseMode: "markdownv2",
			}

			if _, err := bot.Send(msg); err != nil {
				recover()
				panic(err)
			}
		}

		recentRecords = newRecords
	}
}

func init() {
	channelName = os.Getenv("TG_CHANNEL_NAME")
	_bot, err := tgbotapi.NewBotAPI(os.Getenv("TG_BOT_TOKEN"))
	if err != nil {
		panic(err)
	}

	bot = _bot

	poll()
}

func main() {
	ticker := time.NewTicker(30 * time.Minute)
	sigs := make(chan os.Signal)

	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)

	var wg sync.WaitGroup
	go func() {
	loop:
		for {
			select {
			case <-sigs:
				break loop
			case <-ticker.C:
				poll()
			}
		}

		fmt.Println("Goodbye.")
		wg.Done()
	}()

	defer ticker.Stop()
	wg.Add(1)
	wg.Wait()
}
