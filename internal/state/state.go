package state

import "image"

const (
	WaitingEmojiCount = "waitingEmojiCount"
	WaitingTitle      = "waitingTitle"
)

type ImageProcessData struct {
	Image      image.Image
	EmojiCount int
}
