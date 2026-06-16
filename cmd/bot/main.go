package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image/png"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TrixiS/emojis_3x_bot/internal/config"
	"github.com/TrixiS/emojis_3x_bot/internal/fsm"
	"github.com/TrixiS/emojis_3x_bot/internal/images"
	"github.com/TrixiS/emojis_3x_bot/internal/phrases"
	"github.com/TrixiS/emojis_3x_bot/internal/state"
	"github.com/TrixiS/goram"
	"github.com/TrixiS/goram/filters"
	"github.com/TrixiS/goram/flood"
	"github.com/TrixiS/goram/handlers"
)

const (
	emojiCountMin         = 1
	emojiCountMax         = 10
	emojiWidth            = 100
	emojiHeigth           = 100
	stickerSetTitleMaxLen = 64
)

var stickerEmojiList = []string{"👍"}

type Config struct {
	BotToken      string `env:"BOT_TOKEN,required"`
	Secret        string `env:"SECRET,required"`
	WebhookURL    string `env:"WEBHOOK_URL,required"`
	ListenAddress string `env:"LISTEN_ADDRESS,required"`
}

func main() {
	cfg := config.Load(&Config{})

	bot := goram.NewBot(goram.BotOptions{
		Token: cfg.BotToken,
		FloodHandler: flood.NewCondHandler(
			func(ctx context.Context, method string, request any, duration time.Duration) {
				slog.Warn("waiting for flood", "method", method, "dur", duration)
			},
		),
	})

	ctx := context.Background()

	err := bot.SetWebhookVoid(ctx, &goram.SetWebhookRequest{
		URL:            cfg.WebhookURL,
		SecretToken:    cfg.Secret,
		AllowedUpdates: []goram.UpdateType{goram.UpdateMessage},
	})

	if err != nil {
		panic(err)
	}

	me, err := bot.GetMe(ctx)

	if err != nil {
		panic(err)
	}

	h := Handlers{
		me:  me,
		fsm: fsm.New(),
	}

	router := h.router()
	http.HandleFunc("/updates", createUpdateHandler(cfg.Secret, bot, router))

	slog.Info("listen", "address", cfg.ListenAddress)

	if err := http.ListenAndServe(cfg.ListenAddress, nil); err != nil {
		panic(err)
	}
}

func createUpdateHandler(secret string, bot *goram.Bot, router *handlers.Router) http.HandlerFunc {
	const secretHeaderKey = "X-Telegram-Bot-Api-Secret-Token"

	return func(w http.ResponseWriter, r *http.Request) {
		secretHeader := r.Header.Get(secretHeaderKey)

		if secret != secretHeader {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		update := goram.Update{}
		err := json.NewDecoder(r.Body).Decode(&update)
		r.Body.Close()

		if err != nil {
			slog.Error("decode update", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		go func() {
			_, err := router.FeedUpdates(
				context.Background(),
				bot,
				[]goram.Update{update},
				handlers.Data{},
			)

			if err != nil {
				slog.Error("handle update", "err", err)
			}
		}()

		w.WriteHeader(http.StatusOK)
	}
}

type Handlers struct {
	me  *goram.User
	fsm *fsm.FSM
}

func (h Handlers) router() *handlers.Router {
	return handlers.NewRouter(handlers.RouterOptions{Name: "root"}).
		Message(h.startHandler, filters.Text("/start")).
		Message(h.imageHandler, imageFilter).
		Message(h.emojiCountHandler, filters.HasText, h.fsm.FilterMessage(state.WaitingEmojiCount)).
		Message(h.titleHandler, filters.HasText, h.fsm.FilterMessage(state.WaitingTitle))
}

func imageFilter(
	ctx context.Context,
	bot *goram.Bot,
	message *goram.Message,
	data handlers.Data,
) (bool, error) {
	if message.Document == nil {
		return false, nil
	}

	if !strings.HasSuffix(strings.ToLower(message.Document.FileName), ".png") {
		return false, bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
			ChatID: message.ChatID(),
			Text:   phrases.InvalidImageMessageText,
		})
	}

	return true, nil
}

func (h Handlers) startHandler(
	ctx context.Context,
	bot *goram.Bot,
	message *goram.Message,
	data handlers.Data,
) error {
	return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
		ChatID: message.ChatID(),
		Text:   phrases.StartMessageText,
	})
}

func (h Handlers) imageHandler(
	ctx context.Context,
	bot *goram.Bot,
	message *goram.Message,
	data handlers.Data,
) error {
	file, err := bot.GetFile(ctx, &goram.GetFileRequest{FileID: message.Document.FileID})

	if err != nil {
		return err
	}

	r, err := bot.OpenFile(ctx, file)

	if err != nil {
		return err
	}

	defer r.Close()

	pngImage, err := png.Decode(r)

	if err != nil {
		return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
			ChatID: message.ChatID(),
			Text:   phrases.InvalidImageMessageText,
		})
	}

	h.fsm.InitState(message.From.ID, fsm.StateContext{
		State: state.WaitingEmojiCount,
		Data: &state.ImageProcessData{
			Image: pngImage,
		},
	})

	return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
		ChatID: message.ChatID(),
		Text:   fmt.Sprintf(phrases.EnterEmojiCountMessageTextFmt, emojiCountMin, emojiCountMax),
	})
}

func (h Handlers) emojiCountHandler(
	ctx context.Context,
	bot *goram.Bot,
	message *goram.Message,
	data handlers.Data,
) error {
	emojiCount, err := strconv.Atoi(message.Text)

	if err != nil || emojiCount < emojiCountMin || emojiCount > emojiCountMax {
		return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
			ChatID: message.ChatID(),
			Text: fmt.Sprintf(
				phrases.InvalidEmojiCountMessageTextFmt,
				emojiCountMin,
				emojiCountMax,
			),
		})
	}

	stateCtx := data[fsm.StateCtxKey].(*fsm.StateContext)
	stateCtx.State = state.WaitingTitle
	stateData := stateCtx.Data.(*state.ImageProcessData)
	stateData.EmojiCount = emojiCount

	return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
		ChatID: message.ChatID(),
		Text:   phrases.EnterStickerSetTitleMessageText,
	})
}

func (h Handlers) titleHandler(
	ctx context.Context,
	bot *goram.Bot,
	message *goram.Message,
	data handlers.Data,
) error {
	if len(message.Text) > stickerSetTitleMaxLen {
		return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
			ChatID: message.ChatID(),
			Text:   phrases.InvalidStickerSetTitleMessageText,
		})
	}

	h.fsm.ClearState(message.Chat.ID)

	err := bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
		ChatID: message.ChatID(),
		Text:   phrases.ImageProcessingMessageText,
	})

	if err != nil {
		return err
	}

	stateCtx := data[fsm.StateCtxKey].(*fsm.StateContext)
	stateData := stateCtx.Data.(*state.ImageProcessData)

	croppedImage, _ := images.Autocrop(stateData.Image)
	resizedImage := images.ResizeImage(croppedImage, emojiWidth*stateData.EmojiCount, emojiHeigth)
	segments := images.SplitByWidth(resizedImage, emojiWidth)

	stickers := make([]goram.InputSticker, len(segments))

	for i, s := range segments {
		buf := &bytes.Buffer{}

		if err := png.Encode(buf, s); err != nil {
			return err
		}

		stickers[i] = goram.InputSticker{
			Sticker: goram.InputFile{
				Reader: goram.NameReader{
					Reader:   buf,
					FileName: fmt.Sprintf("sticker%d.png", i),
				},
			},
			Format:    "static",
			EmojiList: stickerEmojiList,
		}
	}

	stickerSetName := makeStickerSetName(h.me.Username)

	err = bot.CreateNewStickerSetVoid(ctx, &goram.CreateNewStickerSetRequest{
		UserID:          message.From.ID,
		Title:           message.Text,
		Name:            stickerSetName,
		Stickers:        stickers,
		StickerType:     "custom_emoji",
		NeedsRepainting: false,
	})

	if err != nil {
		return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
			ChatID: message.ChatID(),
			Text:   phrases.FailedToCreateStickerSetMessageText,
		})
	}

	return bot.SendMessageVoid(ctx, &goram.SendMessageRequest{
		ChatID: message.ChatID(),
		Text:   fmt.Sprintf(phrases.CreateStickerSetMessageTextFmt, stickerSetName),
	})
}

func makeStickerSetName(username string) string {
	const sep = "_by_"

	nameBuilder := strings.Builder{}
	nameBuilder.Grow(stickerSetTitleMaxLen)
	writeRandomString(&nameBuilder, stickerSetTitleMaxLen-len(sep)-len(username))
	nameBuilder.WriteString(sep)
	nameBuilder.WriteString(username)
	return nameBuilder.String()
}

func writeRandomString(w interface{ WriteByte(byte) error }, n int) {
	const (
		letters  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digits   = "0123456789"
		alphabet = letters + digits
	)

	firstLetter := letters[getRandomInt(len(letters))]
	w.WriteByte(firstLetter)

	for i := 1; i < n; i++ {
		idx := getRandomInt(len(alphabet))
		w.WriteByte(alphabet[idx])
	}
}

func getRandomInt(max int) int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))

	if err != nil {
		panic(err)
	}

	return n.Int64()
}
