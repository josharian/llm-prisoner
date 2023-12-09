package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sashabaranov/go-openai"
)

var (
	flagIterations = flag.Int("iters", 1, "number of games to play between each pair of players")
	flagUse4       = flag.Bool("4", false, "use GPT-4 instead of GPT-3.5 (expensive!)")
	flagMinRounds  = flag.Int("min-rounds", 5, "minimum number of rounds to play")
	flagMaxRounds  = flag.Int("max-rounds", 8, "maximum number of rounds to play")
	flagRoundDecay = flag.Int("round-decay", 2, "decay denominator for number of round; after min-round, the probability of playing another round at each round is 1-1/round-decay")
	flagMinChatLen = flag.Int("min-chat-len", 2, "minimum number of chat messages, recommended to be at least 2 to avoid huge info asymmetry")
	flagMaxChatLen = flag.Int("max-chat-len", 4, "maximum number of chat messages, recommended to be at most 4 to avoid inane chats")
	flagChatDecay  = flag.Int("chat-decay", 2, "decay denominator for number of chat messages; after min-chat-len, the probability of another chat message after each chat is 1-1/chat-len-decay")
)

type tournament struct {
	cli   *openai.Client
	prng  *rand.Rand
	model string
}

// Player represents a participant in the tournament.
type Player struct {
	Name  string
	Code  string
	Score float64

	cli   *openai.Client
	model string
}

// playTournament iterates over all pairs of players and updates their scores.
func (t *tournament) play(players []*Player) error {
	// for better entertainment, shuffle the players
	list0 := append(players[:0:0], players...)
	list1 := append(players[:0:0], players...)
	t.prng.Shuffle(len(list0), func(i, j int) {
		list0[i], list0[j] = list0[j], list0[i]
	})
	t.prng.Shuffle(len(list1), func(i, j int) {
		list1[i], list1[j] = list1[j], list1[i]
	})

	for iters := 0; iters < *flagIterations; iters++ {
		for _, p0 := range list0 {
			for _, p1 := range list1 {
				fmt.Println("--- ", p0.Name, " ---")
				fmt.Println(p0.Code)
				fmt.Println("--- ", p1.Name, " ---")
				fmt.Println(p1.Code)
				var s0, s1, rounds int
				for {
					var err error
					s0, s1, rounds, err = t.compete(context.Background(), [2]*Player{p0, p1})
					if err != nil {
						fmt.Println("error:", err)
						fmt.Println("sleeping for 1 minute... (TODO: exponential backoff)")
						time.Sleep(1 * time.Minute)
						continue
					}
					break
				}
				p0.Score += float64(s0) / float64(rounds)
				p1.Score += float64(s1) / float64(rounds)
				fmt.Println()
				fmt.Println()
			}
		}
	}
	return nil
}

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Println("Usage: go run main.go <path_to_players_directory>")
		os.Exit(1)
	}

	playersDir := flag.Arg(0)

	cli := openai.NewClient(os.Getenv("OPENAI"))

	t := &tournament{
		cli:   cli,
		prng:  rand.New(rand.NewSource(int64(time.Now().UnixNano()))), // TODO: use better prng
		model: openai.GPT3Dot5Turbo,
	}
	if *flagUse4 {
		t.model = openai.GPT4TurboPreview
	}

	// Initialize players
	var players []*Player
	err := filepath.Walk(playersDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			code, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			player := &Player{
				Name:  info.Name(),
				Score: 0,
				cli:   cli,
				Code:  string(code),
				model: t.model,
			}
			players = append(players, player)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to initialize players: %v", err)
	}

	// Play the tournament
	err = t.play(players)
	if err != nil {
		log.Fatalf("Failed to play tournament: %v", err)
	}

	// Sort players by score
	sort.Slice(players, func(i, j int) bool {
		return players[i].Score > players[j].Score
	})
	// Print the final scores
	for _, player := range players {
		fmt.Printf("%s: %d\n", player.Name, int(player.Score))
	}
}
