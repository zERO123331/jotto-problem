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

const workers = 20
const expected = 538

type Word struct {
	Word     string
	Anagrams []string
	Letters  map[rune]bool
}

type Anagram struct {
	Word        string
	SortLetters string
}

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("Starting", "Workers", workers, "Expected Pairs", expected)
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
	wordschannel := make(chan []Word, 100)
	anagramChannel := make(chan []Anagram, 100)
	jobChannel := make(chan int, len(okwords))
	for i := 0; i < workers; i++ {
		go filterWords(wordschannel, anagramChannel, jobChannel, okwords)
	}

	for i := 0; i < len(okwords); i++ {
		jobChannel <- i
	}

	close(jobChannel)

	var newOkWords []Word
	var anagrams []Anagram
	i := 0
	for i < workers {
		data := <-wordschannel
		if len(data) != 0 {
			newOkWords = append(newOkWords, data...)
		}

		recievedAnagrams := <-anagramChannel
		anagrams = append(anagrams, recievedAnagrams...)
		i++
	}

	close(wordschannel)
	close(anagramChannel)

	okwords = newOkWords
	for _, anagram := range anagrams {
		anagram.SortLetters = sortLetters(anagram.Word)
		for i, word := range okwords {
			if word.Word == anagram.SortLetters {
				word.Anagrams = append(word.Anagrams, anagram.Word)
				newOkWords[i] = word
				break
			}
		}
	}
	seenAnagrams := make(map[string]bool)
	for _, word := range okwords {
		for _, anagram := range word.Anagrams {
			if seenAnagrams[anagram] {
				panic("Anagram already seen")
			}
			seenAnagrams[anagram] = true
		}
	}

	slices.SortFunc(okwords, func(a, b Word) int {
		return strings.Compare(a.Word, b.Word)
	})

	logger.Info("Finished setting up Words and Anagrams", "Duration", time.Since(starttime).String())
	logger.Info("Starting Pair search with parameters:", "Word count", len(okwords), "Threads", workers, `Expected Pairs`, expected)
	searchStartTime := time.Now()
	ch := make(chan [][]Word, workers)
	jobChannel = make(chan int, len(okwords))
	for i := 0; i < workers; i++ {
		go finishWordlistWorker(ch, jobChannel, okwords)
	}
	for i := 0; i < len(okwords); i++ {
		jobChannel <- i
	}
	close(jobChannel)
	var wordpairs [][]Word
	pairsReceived := 0
	for i := 0; i < workers; i++ {
		data := <-ch
		if len(data) != 0 {
			pairsReceived += len(data)
			averageTime := time.Since(searchStartTime).Seconds() / float64(pairsReceived)
			wordpairs = append(wordpairs, data...)
			estimatedTime := timeRemaining(expected, len(wordpairs), workers, time.Since(searchStartTime))
			logger.Info("Pair(s) received", "Amount Received", len(data), "Total Pairs", len(wordpairs), "Expected", expected, "Average Time", fmt.Sprintf("%f Seconds/Pair", averageTime), "Estimated Time Remaining", estimatedTime.String(), "Estimated Total Time", (estimatedTime + time.Since(searchStartTime)).String(), "Workers working", fmt.Sprintf("%d/%d", workers-i, workers))
		} else {
			logger.Error("A worker has finished but no data was received")
		}
	}
	logger.Info("Finished making Pairs", "Duration", time.Since(starttime).String())
	logger.Info("Pairs", "Received/Expected", fmt.Sprintf("%d/%d", len(wordpairs), expected))
	close(ch)

	logger.Info("Starting Anagram search")

	solutions := assembleSolutions(wordpairs)

	logger.Info("Solutions with anagrams", "Solutions", len(solutions))

	slices.SortFunc(solutions, func(a, b []string) int {
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
	err = outputfile.Truncate(0)
	if err != nil {
		logger.Error("Error truncating file", "Error", err)
		panic(err)
	}

	defer outputfile.Close()

	writeErrors := 0
	for _, pair := range solutions {
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

func filterWords(resultChannel chan<- []Word, anagramChannel chan<- []Anagram, jobChannel <-chan int, okwords []Word) {
	var newokwords []Word
	var anagrams []Anagram
	for j := range jobChannel {
		anagram := Anagram{
			Word:        okwords[j].Word,
			SortLetters: sortLetters(okwords[j].Word),
		}
		if !hasanagram(j, okwords) {
			word := okwords[j]
			word.Word = anagram.SortLetters
			newokwords = append(newokwords, word)
		}
		anagrams = append(anagrams, anagram)
	}
	resultChannel <- newokwords
	anagramChannel <- anagrams
}

func finishWordlistWorker(resultChannel chan<- [][]Word, jobChannel <-chan int, okwords []Word) {
	var newokwords [][]Word
	for j := range jobChannel {
		newokwords = append(newokwords, finishWordList(j, okwords)...)
	}
	resultChannel <- newokwords
}

func finishWordList(index int, okwords []Word) [][]Word {
	wordlist := []Word{
		okwords[index],
	}
	var addedLetters map[rune]bool
	addedLetters = addLetters(addedLetters, wordlist[0].Letters)
	var foundcombinations [][]Word
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
					foundcombinations = append(foundcombinations, []Word{
						wordlist[0], word2, word3, word4, word5,
					})
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
	previousWord := []byte(prevword.Word)
	if wordBytes[0] <= previousWord[0] {
		return false
	}

	for letter, _ := range word.Letters {
		if word.Letters[letter] == l[letter] {
			return false
		}
	}

	return true
}

func sortLetters(word string) string {
	letterOrder := []rune{
		'e', 's', 'i', 'a', 'r', 'n', 't', 'o', 'l', 'c', 'd', 'u', 'g', 'p', 'm', 'h', 'b', 'y', 'f', 'v', 'k', 'w', 'z', 'x', 'j', 'q',
	}
	var sortedLetters []rune
	for _, sortedLetter := range letterOrder {
		for _, letter := range word {
			if letter == sortedLetter {
				sortedLetters = append(sortedLetters, letter)
			}
		}
	}
	return string(sortedLetters)

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

func assembleSolutions(wordPairs [][]Word) [][]string {
	solutions := [][]string{}
	for _, pair := range wordPairs {
		words := make([][]string, len(pair))
		for i, word := range pair {
			var anagrams []string
			if word.Anagrams == nil {
				panic("Anagrams is nil")
			}
			for _, anagram := range word.Anagrams {
				anagrams = append(anagrams, anagram)
			}
			words[i] = anagrams
		}
		for _, word1 := range words[0] {
			for _, word2 := range words[1] {
				for _, word3 := range words[2] {
					for _, word4 := range words[3] {
						for _, word5 := range words[4] {
							solutions = append(solutions, []string{word1, word2, word3, word4, word5})
						}
					}
				}
			}
		}
	}
	for i, solution := range solutions {
		slices.SortFunc(solution, func(a, b string) int {
			return strings.Compare(a, b)
		})
		solutions[i] = solution
	}
	return solutions
}
