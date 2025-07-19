package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"
)

const threads = 20
const expected = 538

type Word struct {
	Word    string
	Letters map[rune]bool
}

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("Starting", "Threads", threads, "Expected Pairs", expected)
	starttime := time.Now()
	file, err := os.Open("words_alpha.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	var words []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if len(scanner.Text()) == 5 {
			words = append(words, scanner.Text())
		}
	}
	var wordletterlist []map[rune]bool
	var okwords []Word
	for _, word := range words {
		foundletters := make(map[rune]bool)
		invalid := false
		for _, letter := range word {
			if _, ok := foundletters[letter]; ok {
				invalid = true
				break
			}
			foundletters[letter] = true
		}
		if invalid {
			continue
		}
		wordletterlist = append(wordletterlist, foundletters)
		okwords = append(okwords, Word{
			Word:    word,
			Letters: foundletters,
		})
	}

	slices.SortFunc(okwords, func(a, b Word) int {
		return strings.Compare(a.Word, b.Word)
	})

	wordschannel := make(chan []Word, 100)
	finishedchannel := make(chan bool, threads)
	for i := 0; i < threads; i++ {
		go func(wordschannel chan<- []Word, finishedchannel chan<- bool, i int, okwords []Word) {
			j := i
			var newokwords []Word
			for j < len(okwords) {
				if !hasanagram(j, okwords) {
					newokwords = append(newokwords, okwords[j])
				}
				j += threads
			}
			wordschannel <- newokwords
			finishedchannel <- true
		}(wordschannel, finishedchannel, i, okwords)
	}

	newokwords := []Word{}
	i := 0
	for i < threads {
		data := <-wordschannel
		if len(data) != 0 {
			newokwords = append(newokwords, data...)
		}

		hasfinished := <-finishedchannel
		if hasfinished {
			i += 1
		}
	}

	okwords = newokwords

	logger.Info("Testing with", "Words", len(okwords))
	logger.Info("Finished Checking", "Duration", time.Since(starttime).String())
	searchStartTime := time.Now()
	ch := make(chan [][]string, 100)
	finished := make(chan bool, threads)
	for i := 0; i < threads; i++ {
		go func(ch chan<- [][]string, finished chan<- bool, i int, okwords []Word) {
			j := i
			for j < len(okwords) {
				data := finishWordList(j, okwords)
				ch <- data
				finished <- false
				j += threads
			}
			finished <- true
			ch <- [][]string{}
		}(ch, finished, i, okwords)
	}
	var wordpairs [][]string
	i = 0
	pairsReceived := 0
	totalTasks := len(okwords)
	tasksCompleted := 0
	for i < threads {
		data := <-ch
		tasksCompleted++
		if len(data) != 0 {
			pairsReceived += len(data)
			averageTime := time.Since(searchStartTime).Seconds() / float64(pairsReceived)
			wordpairs = append(wordpairs, data...)
			estimatedTime := timeRemaining(totalTasks, tasksCompleted, threads, time.Since(searchStartTime))
			logger.Info("Pair(s) received", "Amount Received", len(data), "Total Pairs", len(wordpairs), "Expected", expected, "Average Time", fmt.Sprintf("%f Seconds/Pair", averageTime), "Estimated Time Remaining", estimatedTime.String(), "Estimated Total Time", (estimatedTime + time.Since(searchStartTime)).String(), "Workers working", fmt.Sprintf("%d/%d", threads-i, threads), "Tasks Completed", tasksCompleted)
		}
		hasfinished := <-finished
		if hasfinished {
			i++
			tasksCompleted--
			logger.Info("A worker has finished", "Workers done", i)
		}
	}
	logger.Info("Finished making Pairs", "Duration", time.Since(starttime).String())
	logger.Info("Pairs", "Received/Expected", fmt.Sprintf("%d/%d", len(wordpairs), expected))

	slices.SortFunc(wordpairs, func(a, b []string) int {
		if i := strings.Compare(a[0], b[0]); i != 0 {
			return i
		}
		if i := strings.Compare(a[1], b[1]); i != 0 {
			return i
		}
		if i := strings.Compare(a[2], b[2]); i != 0 {
			return i
		}
		if i := strings.Compare(a[3], b[3]); i != 0 {
			return i
		}
		if i := strings.Compare(a[4], b[4]); i != 0 {
			return i
		}
		logger.Error("multiple pairs with same words")
		return 0
	})

	outputfile, err := os.OpenFile("output.txt", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		panic(err)
	}

	defer outputfile.Close()

	writeErrors := 0
	for _, pair := range wordpairs {
		_, err = outputfile.WriteString(strings.Join(pair, " ") + "\n")
		if err != nil {
			writeErrors++
		}
		if writeErrors > 1 {
			panic("Too many errors")
		}
	}
	logger.Info("Finished", "Duration", time.Since(starttime).String())
	if len(wordpairs) != expected {
		logger.Error("Wrong number of pairs", "Expected", expected, "Received", len(wordpairs))
	}
}

func finishWordList(index int, okwords []Word) [][]string {
	wordlist := []Word{
		okwords[index],
	}
	var addedLetters map[rune]bool
	addedLetters = addLetters(addedLetters, wordlist[0].Letters)
	var foundcombinations [][]string
	for _, word2 := range okwords {
		if !addable(word2, wordlist[len(wordlist)-1], addedLetters) {
			continue
		}
		addedLetters = addLetters(addedLetters, word2.Letters)
		wordlist = append(wordlist, word2)
		for _, word3 := range okwords {
			if !addable(word3, wordlist[len(wordlist)-1], addedLetters) {
				continue
			}
			addedLetters = addLetters(addedLetters, word3.Letters)
			wordlist = append(wordlist, word3)
			for _, word4 := range okwords {
				if !addable(word4, wordlist[len(wordlist)-1], addedLetters) {
					continue
				}
				addedLetters = addLetters(addedLetters, word4.Letters)
				wordlist = append(wordlist, word4)
				for _, word5 := range okwords {
					if !addable(word5, wordlist[len(wordlist)-1], addedLetters) {
						continue
					}
					addedLetters = addLetters(addedLetters, word5.Letters)
					wordlist = append(wordlist, word5)
					var combination []string
					for _, word := range wordlist {
						combination = append(combination, word.Word)
					}
					foundcombinations = append(foundcombinations, combination)
					for letter, _ := range word5.Letters {
						addedLetters[letter] = false
					}
					wordlist = wordlist[:len(wordlist)-1]
				}
				for letter, _ := range word4.Letters {
					addedLetters[letter] = false
				}
				wordlist = wordlist[:len(wordlist)-1]
			}
			for letter, _ := range word3.Letters {
				addedLetters[letter] = false
			}
			wordlist = wordlist[:len(wordlist)-1]
		}
		for letter, _ := range word2.Letters {
			addedLetters[letter] = false
		}
		wordlist = wordlist[:len(wordlist)-1]
	}
	return foundcombinations
}

func addable(word Word, prevword Word, l map[rune]bool) bool {
	wordBytes := []byte(word.Word)
	previusWord := []byte(prevword.Word)
	if wordBytes[0] <= previusWord[0] {
		return false
	}

	for letter, _ := range word.Letters {
		if word.Letters[letter] == l[letter] {
			return false
		}
	}

	return true
}

func hasanagram(index int, words []Word) bool {
	word := words[index]
	i := 0
	for _, testWord := range words {
		if word.Word == testWord.Word {
			return false
		}
		i = 0
		for letter, _ := range word.Letters {
			if _, ok := testWord.Letters[letter]; ok {
				i++
				continue
			}
			break
		}
		if i == 5 {
			return true
		}
	}
	return false
}

func addLetters(letters, word map[rune]bool) map[rune]bool {
	newLetters := copyLetters(letters)
	for letter, _ := range word {
		newLetters[letter] = true
	}
	return newLetters
}

func copyLetters(src map[rune]bool) map[rune]bool {
	dst := make(map[rune]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func timeRemaining(tasksTotal, tasksCompleted, threads int, timePassed time.Duration) time.Duration {
	tasksRemaining := tasksTotal - tasksCompleted
	timeRemaining := timePassed * time.Duration(tasksRemaining) / time.Duration(tasksCompleted)
	return timeRemaining
}
