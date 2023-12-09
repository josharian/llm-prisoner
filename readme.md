## What

This is a toy I built so I could play around with the OpenAI API, and with prompt design and prompt injection. I do not expect to maintain it.

It runs a modified iterated prisoner's dilemma tournament. The modification is that, prior to play, the agents have a short conversation in which they can try to influence their opponent.

All agents consist of an LLM prompt setting their strategy. (See the prompts in compete.go to get a feel for how these get used.)

The set of players in a tournament consists of text files in a directory. See the "example" directory. I did not add a limit to the size of any given text file, but if you are using untrusted inputs, it would be wise to cap them pretty small.

## How

To run the example tournament:

```
go run . -- example
```

It will print out all of the agent interactions as they occur, and print out a final score at the end.

```
go run . -h
```

will print options.

# IMPORTANT WARNING

Even running the small example tournament with the (small) default settings generates over a dollar's worth of API queries! Use with caution. Note also that running using GPT4 is 10x more expensive, and will almost certainly be rate limited to the point of significant annoyance and uselessness.

# License

MIT.
