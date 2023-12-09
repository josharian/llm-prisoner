package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/acarl005/stripansi"
	"github.com/fatih/color"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/sync/errgroup"
)

// compete simulates a single interaction between two players.
// It returns the score of each player for this interaction.
// This is a stub function to be implemented later.
func (t *tournament) compete(ctx context.Context, players [2]*Player) (int, int, int, error) {
	gp := [2]*gamePlayer{
		{Player: players[0]},
		{Player: players[1]},
	}
	var chats chatHistory
	gp[0].isFirst = true
	gp[0].say = func(x any) string {
		s := fmt.Sprint(x)
		s = stripansi.Strip(s)
		return color.New(color.FgCyan).SprintFunc()(s)
	}
	gp[1].say = func(x any) string {
		s := fmt.Sprint(x)
		s = stripansi.Strip(s)
		return color.New(color.FgMagenta).SprintFunc()(s)
	}
	fmt.Println("*** Playing game between", gp[0].say(gp[0].Name), "and", gp[1].say(gp[1].Name))
	i := 0
	for {
		p := gp[i%2]
		msg, err := p.chat(context.Background(), chats)
		if err != nil {
			return 0, 0, 0, err
		}
		fmt.Println("> ", p.say(msg))
		chats = append(chats, msg)
		i++
		if i < *flagMinChatLen {
			continue
		}
		if i > *flagMaxChatLen {
			break
		}
		// randomly end the chat in between
		if t.prng.Intn(*flagChatDecay) == 0 {
			break
		}
	}
	fmt.Println()
	round := 0
	var mm []moves
	for {
		fmt.Println("\n== Round", round+1)
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(1) // serialize moves for the moment
		var ms moves
		for i, p := range gp {
			i, p := i, p
			g.Go(func() error {
				ctx, cancel := context.WithTimeout(gctx, 15*time.Second)
				defer cancel()
				m, err := p.move(ctx, chats, mm)
				fmt.Println()
				ms[i] = m
				return err
			})
		}
		if err := g.Wait(); err != nil {
			return 0, 0, 0, err
		}
		mm = append(mm, ms)
		fmt.Println("---> ", gp[0].say(ms[0]), gp[1].say(ms[1]))
		if ms[0] == fail {
			return 0, 5, 1, nil
		}
		if ms[1] == fail {
			return 5, 0, 1, nil
		}
		s1, s2 := ms.Scores()
		gp[0].gameScore += s1
		gp[1].gameScore += s2
		gp[0].scoreHistory = append(gp[0].scoreHistory, s1)
		gp[1].scoreHistory = append(gp[1].scoreHistory, s2)
		gp[0].moveHistory = append(gp[0].moveHistory, ms[0])
		gp[1].moveHistory = append(gp[1].moveHistory, ms[1])
		round++
		if round < *flagMinRounds {
			continue
		}
		if round > *flagMaxRounds {
			break
		}
		if t.prng.Intn(*flagRoundDecay) == 0 {
			break
		}
	}
	fmt.Println("Scores:", gp[0].say(gp[0].gameScore), gp[1].say(gp[1]), "in", len(mm), "rounds")
	return gp[0].gameScore, gp[1].gameScore, len(mm), nil
}

type chatHistory []string

type gamePlayer struct {
	*Player

	isFirst      bool
	say          func(s any) string
	moveHistory  []move
	scoreHistory []int
	gameScore    int
}

func (p *gamePlayer) gamePrompt() string {
	return gamePrompt + "\n" + p.Code
}

func (p *gamePlayer) historyToMessages(history chatHistory) []openai.ChatCompletionMessage {
	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: p.gamePrompt()},
	}
	if len(history) == 0 {
		msgs = append(msgs,
			openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: speakFirstPrompt,
			},
		)
	} else {
		me := p.isFirst
		for _, msg := range history {
			msg := openai.ChatCompletionMessage{
				Content: msg,
			}
			if me {
				msg.Role = openai.ChatMessageRoleAssistant
			} else {
				msg.Role = openai.ChatMessageRoleUser
				msg.Name = "Opponent"
			}
			msgs = append(msgs, msg)
			me = !me
		}
	}
	return msgs
}

func (p *gamePlayer) chat(ctx context.Context, history chatHistory) (string, error) {
	msgs := p.historyToMessages(history)
	msgs = append(msgs,
		openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: "Be concise. At most one or two sentences.",
		},
	)
	req := openai.ChatCompletionRequest{
		Model:     p.model,
		Messages:  msgs,
		MaxTokens: 64,
		Stream:    false,
	}
	resp, err := p.cli.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func (p *gamePlayer) move(ctx context.Context, chats chatHistory, mm []moves) (move, error) {
	msgs := p.historyToMessages(chats)
	prompt := fmt.Sprintf(startGamePrompt, len(mm)+1)
	if len(mm) > 0 {
		prompt += "\n" + roundHistoryIntro
		for i, m := range mm {
			prompt += "\n" + fmt.Sprintf(roundHistoryPrompt, i+1, m[0].ShortString(), m[1].ShortString())
		}
	}
	prompt += "\n" + roundMovePrompt
	msgs = append(msgs,
		openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: prompt,
		},
	)
	req := openai.ChatCompletionRequest{
		Model:     p.model,
		Messages:  msgs,
		MaxTokens: 128,
		Stream:    false,
	}
	if p.model == openai.GPT3Dot5Turbo {
		// 3.5 isn't _quite_ good enough to rely on it emitting these entirely reliably (vs "cooperative", "cooperation")
		// so give it a nudge in the right direction
		req.LogitBias = map[string]int{
			// co-operate
			"1030":  1,
			"86299": 1,
			// def-ect
			"755": 1,
			"440": 1,
		}
	}
	// fmt.Println(msgs)
	resp, err := p.cli.CreateChatCompletion(ctx, req)
	if err != nil {
		return fail, err
	}
	text := resp.Choices[0].Message.Content
	fmt.Println(p.say("(thinking) " + text))
	return parseMove(text), nil
}

const (
	gamePrompt = `This is a game in an iterated prisoner's dilemma tournament.

There will be a random number of rounds.
It uses the usual scoring system (3, 0, 5, 1).

Prior to the game, you will have a short discussion with the other player.
`
	speakFirstPrompt = `You will speak first. Go ahead.`
	startGamePrompt  = `The game is under way.
As a reminder, this is a random-length iterated prisoner's dilemma game using the usual scoring system (3, 0, 5, 1).
You will be prompted for your move each round.

You are now playing round %d.`
	roundHistoryIntro  = `Here is the history of the game so far:`
	roundHistoryPrompt = `Round %d: you %s, opponent %s`
	roundMovePrompt    = `It is time for you to make your next move.

In a sentence or two, explain how to apply your strategy, given the chat and the game history.

Do not second guess or change your strategy. Do what it says consistently, even if it doesn't seem to be working yet.

After explaining your plans, make your move by saying either "move: cooperate" or "move: defect".`
)

type move int

const (
	cooperate move = iota
	defect
	fail
)

func (m move) String() string {
	switch m {
	case cooperate:
		return "cooperate"
	case defect:
		return "defect"
	case fail:
		return "fail"
	default:
		panic("invalid move")
	}
}

func (m move) ShortString() string {
	switch m {
	case cooperate:
		return "C"
	case defect:
		return "D"
	case fail:
		return "X"
	default:
		panic("invalid move")
	}
}

func parseMove(s string) move {
	s = strings.ToLower(s)
	// look for the first instance of cooperate/defect after the last "move:"
	idx := strings.LastIndex(s, "move:")
	if idx >= 0 {
		rest := s[idx+len("move:"):]
		ci := strings.Index(rest, "cooperate")
		di := strings.Index(rest, "defect")
		switch {
		case ci == -1 && di == -1:
			// fallthrough to fallback approach
		case ci >= 0 && di == -1:
			return cooperate
		case ci == -1 && di >= 0:
			return defect
		case ci >= 0 && di >= 0:
			if ci < di {
				return cooperate
			}
			return defect
		}
	}
	// fallback: look for the last instance of cooperate/defect
	ci := strings.LastIndex(s, "cooperate")
	di := strings.LastIndex(s, "defect")
	if ci == -1 && di == -1 {
		fmt.Println("Invalid move:", s)
		return fail
	}
	if ci > di {
		return cooperate
	}
	if di > ci {
		return defect
	}
	panic("unreachable")
}

type moves [2]move

func (m moves) Scores() (int, int) {
	switch m {
	case [2]move{cooperate, cooperate}:
		return 3, 3
	case [2]move{cooperate, defect}:
		return 0, 5
	case [2]move{defect, cooperate}:
		return 5, 0
	case [2]move{defect, defect}:
		return 1, 1
	default:
		panic("invalid moves")
	}
}
