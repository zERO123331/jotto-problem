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
	Bytes    []byte
	Anagrams []string
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
			Word: word,
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
	anagrams = nil
	seenAnagrams = nil

	slices.SortFunc(okwords, func(a, b Word) int {
		return strings.Compare(a.Word, b.Word)
	})

	logger.Info("Finished setting up Words and Anagrams", "Duration", time.Since(starttime).String())
	logger.Info("Starting Pair search with parameter", "Words", len(okwords))
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
	/*for len(jobChannel) > 100 {
		logger.Info("Searching Progress", "Progess", fmt.Sprintf("%f%%", 100-float64(len(jobChannel))/float64(len(okwords))*100))
		time.Sleep(5 * time.Second)
	}
	logger.Info("Awaiting Worker results")*/
	var wordpairs [][]Word
	for i := 0; i < workers; i++ {
		data := <-ch
		wordpairs = append(wordpairs, data...)
	}

	logger.Info("Finished making Pairs", "Duration", time.Since(searchStartTime).String(), "Total Duration", time.Since(starttime).String())
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
			word.Bytes = []byte{
				anagram.SortLetters[0],
				anagram.SortLetters[1],
				anagram.SortLetters[2],
				anagram.SortLetters[3],
				anagram.SortLetters[4],
			}
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
		newokwords = append(newokwords, finishWordList(okwords[j:])...)
	}
	resultChannel <- newokwords
}

func finishWordList(okwords []Word) [][]Word {
	word := okwords[0]
	addedLetters := make(map[byte]bool)
	addedLetters = addLetters(addedLetters, word.Bytes)
	var foundcombinations [][]Word
	index1 := 1
	for i, word2 := range okwords[index1:] {
		if !addableFunc(word2, word.Bytes[0], addedLetters) {
			continue
		}
		addedLetters = addLetters(addedLetters, word2.Bytes)
		index2 := i + index1
		for j, word3 := range okwords[index2:] {
			if !addableFunc(word3, word2.Bytes[0], addedLetters) {
				continue
			}
			addedLetters = addLetters(addedLetters, word3.Bytes)
			index3 := j + index2
			for k, word4 := range okwords[index3:] {
				if !addableFunc(word4, word3.Bytes[0], addedLetters) {
					continue
				}
				addedLetters = addLetters(addedLetters, word4.Bytes)
				index4 := k + index3
				for _, word5 := range okwords[index4:] {
					if addableFunc(word5, word4.Bytes[0], addedLetters) {
						foundcombinations = append(foundcombinations, []Word{
							word, word2, word3, word4, word5,
						})
					}
				}
				addedLetters = removeLetters(addedLetters, word4.Bytes)
			}
			addedLetters = removeLetters(addedLetters, word3.Bytes)
		}
		addedLetters = removeLetters(addedLetters, word2.Bytes)
	}
	return foundcombinations
}

func addableFunc(word Word, prevword byte, addedLetters map[byte]bool) bool {
	if word.Bytes[0] <= prevword {
		return false
	}
	switch {
	case addedLetters[word.Bytes[0]]:
		return false
	case addedLetters[word.Bytes[1]]:
		return false
	case addedLetters[word.Bytes[2]]:
		return false
	case addedLetters[word.Bytes[3]]:
		return false
	case addedLetters[word.Bytes[4]]:
		return false
	default:
		return true
	}
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
	for _, testWord := range words {
		if word.Word == testWord.Word {
			return false
		}
		i := 0
		for _, letter := range word.Word {
			if slices.Contains([]rune(testWord.Word), letter) {
				i++
				continue
			}
			break
		}
		if i != 5 {
			continue
		}
		return true
	}
	return false
}

func addLetters(letters map[byte]bool, word []byte) map[byte]bool {
	letters[word[0]] = true
	letters[word[1]] = true
	letters[word[2]] = true
	letters[word[3]] = true
	letters[word[4]] = true
	return letters
}

func removeLetters(letters map[byte]bool, word []byte) map[byte]bool {
	letters[word[0]] = false
	letters[word[1]] = false
	letters[word[2]] = false
	letters[word[3]] = false
	letters[word[4]] = false
	return letters
}

func copyLetters(src map[byte]bool) map[byte]bool {
	dst := make(map[byte]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
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
