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

type Node struct {
	Word     string
	Bytes    []byte
	Leafs    []*Node
	Anagrams []string
}

type Anagram struct {
	Word        string
	SortLetters string
}

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("Starting", "Workers", workers, "Expected Pairs", expected)
	startTime := time.Now()
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

	var wordLetterList []map[rune]bool
	var okWords []*Node
	for _, word := range words {
		foundLetters := make(map[rune]bool)
		invalid := false
		for _, letter := range word {
			if _, ok := foundLetters[letter]; ok {
				invalid = true
				break
			}
			foundLetters[letter] = true
		}
		if invalid {
			continue
		}
		wordLetterList = append(wordLetterList, foundLetters)
		okWords = append(okWords, &Node{
			Word: word,
		})
	}
	wordsChannel := make(chan []*Node, 100)
	anagramChannel := make(chan []Anagram, 100)
	jobChannel := make(chan int, len(okWords))
	for i := 0; i < workers; i++ {
		go filterWords(wordsChannel, anagramChannel, jobChannel, okWords)
	}

	for i := 0; i < len(okWords); i++ {
		jobChannel <- i
	}

	close(jobChannel)

	var newOkWords []*Node
	var anagrams []Anagram
	i := 0
	for i < workers {
		data := <-wordsChannel
		if len(data) != 0 {
			newOkWords = append(newOkWords, data...)
		}

		receivedAnagrams := <-anagramChannel
		anagrams = append(anagrams, receivedAnagrams...)
		i++
	}

	close(wordsChannel)
	close(anagramChannel)

	okWords = newOkWords
	for _, anagram := range anagrams {
		anagram.SortLetters = sortLetters(anagram.Word)
		for i, word := range okWords {
			if word.Word == anagram.SortLetters {
				word.Anagrams = append(word.Anagrams, anagram.Word)
				newOkWords[i] = word
				break
			}
		}
	}
	seenAnagrams := make(map[string]bool)
	for _, word := range okWords {
		for _, anagram := range word.Anagrams {
			if seenAnagrams[anagram] {
				panic("Anagram already seen")
			}
			seenAnagrams[anagram] = true
		}
	}
	anagrams = nil
	seenAnagrams = nil
	slices.SortFunc(okWords, func(a, b *Node) int {
		return strings.Compare(a.Word, b.Word)
	})

	logger.Info("Assembling Leafs")

	leafJobChannel := make(chan int, len(okWords))
	wordsChannel = make(chan []*Node, workers)

	for i := 0; i < workers; i++ {
		go assembleLeafWorker(wordsChannel, leafJobChannel, okWords)
	}
	for i := 0; i < len(okWords); i++ {
		leafJobChannel <- i
	}
	close(leafJobChannel)
	var receivedElements []*Node
	for i := 0; i < workers; i++ {
		data := <-wordsChannel
		receivedElements = append(receivedElements, data...)
	}
	okWords = receivedElements
	close(wordsChannel)
	logger.Info("Finished assembling Leafs")

	slices.SortFunc(okWords, func(a, b *Node) int {
		return strings.Compare(a.Word, b.Word)
	})

	logger.Info("Finished setting up Words and Anagrams", "Duration", time.Since(startTime).String())
	logger.Info("Starting Pair search with parameter", "Words", len(okWords))
	searchStartTime := time.Now()
	ch := make(chan [][]*Node, workers)
	jobChannel = make(chan int, len(okWords))
	for i := 0; i < workers; i++ {
		go finishWordlistWorker(ch, jobChannel, okWords)
	}
	for i := 0; i < len(okWords); i++ {
		jobChannel <- i
	}
	close(jobChannel)
	/*for len(jobChannel) > 100 {
		logger.Info("Searching Progress", "Progress", fmt.Sprintf("%f%%", 100-float64(len(jobChannel))/float64(len(okWords))*100))
		time.Sleep(5 * time.Second)
	}
	logger.Info("Awaiting Worker results")*/
	var wordPairs [][]*Node
	for i := 0; i < workers; i++ {
		data := <-ch
		wordPairs = append(wordPairs, data...)
	}

	logger.Info("Finished making Pairs", "Duration", time.Since(searchStartTime).String(), "Total Duration", time.Since(startTime).String())
	logger.Info("Pairs", "Received/Expected", fmt.Sprintf("%d/%d", len(wordPairs), expected))
	close(ch)

	logger.Info("Starting Anagram search")

	solutions := assembleSolutions(wordPairs)

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

	outputFile, err := os.OpenFile("output.txt", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		panic(err)
	}
	err = outputFile.Truncate(0)
	if err != nil {
		logger.Error("Error truncating file", "Error", err)
		panic(err)
	}

	defer outputFile.Close()

	writeErrors := 0
	for _, pair := range solutions {
		_, err = outputFile.WriteString(strings.Join(pair, " ") + "\n")
		if err != nil {
			writeErrors++
		}
		if writeErrors > 1 {
			panic("Too many errors")
		}
	}
	logger.Info("Finished", "Duration", time.Since(startTime).String())
	if len(wordPairs) != expected {
		logger.Error("Wrong number of pairs", "Expected", expected, "Received", len(wordPairs))
	}
}

func filterWords(resultChannel chan<- []*Node, anagramChannel chan<- []Anagram, jobChannel <-chan int, okWords []*Node) {
	var newOkWords []*Node
	var anagrams []Anagram
	for j := range jobChannel {
		word := *okWords[j]
		if !hasAnagram(j, okWords) {
			word.Word = sortLetters(word.Word)
			word.Bytes = []byte{
				word.Word[0], word.Word[1], word.Word[2], word.Word[3], word.Word[4],
			}
			newOkWords = append(newOkWords, &word)
		}
		anagram := Anagram{
			Word:        okWords[j].Word,
			SortLetters: word.Word,
		}
		anagrams = append(anagrams, anagram)
	}
	resultChannel <- newOkWords
	anagramChannel <- anagrams
}

func assembleLeafWorker(resultChannel chan<- []*Node, jobChannel <-chan int, okWords []*Node) {
	var checkedParentLeafCombo []*Node
	for j := range jobChannel {
		node := okWords[j]
		node.LeafAdder(okWords[j:])
		checkedParentLeafCombo = append(checkedParentLeafCombo, node)
	}
	resultChannel <- checkedParentLeafCombo
}

func finishWordlistWorker(resultChannel chan<- [][]*Node, jobChannel <-chan int, okWords []*Node) {
	var newOkWords [][]*Node
	for j := range jobChannel {
		newOkWords = append(newOkWords, finishWordList(okWords[j:])...)
	}
	resultChannel <- newOkWords
}

func finishWordList(okWords []*Node) [][]*Node {
	word := okWords[0]
	addedLetters := make(map[byte]bool)
	addedLetters = addLetters(addedLetters, word.Bytes)
	var foundCombinations [][]*Node
	for _, word2 := range word.Leafs {
		if !addableFunc(word2, addedLetters) {
			continue
		}
		addedLetters = addLetters(addedLetters, word2.Bytes)
		for _, word3 := range word2.Leafs {
			if !addableFunc(word3, addedLetters) {
				continue
			}
			addedLetters = addLetters(addedLetters, word3.Bytes)
			for _, word4 := range word3.Leafs {
				if !addableFunc(word4, addedLetters) {
					continue
				}
				addedLetters = addLetters(addedLetters, word4.Bytes)
				for _, word5 := range word4.Leafs {
					if addableFunc(word5, addedLetters) {
						foundCombinations = append(foundCombinations, []*Node{
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
	return foundCombinations
}

func addableFunc(word *Node, addedLetters map[byte]bool) bool {
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

func hasAnagram(index int, words []*Node) bool {
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

func assembleSolutions(wordPairs [][]*Node) [][]string {
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

func (n *Node) LeafAdder(nodes []*Node) {
	for _, node := range nodes {
		if n.isLeafOk(node) {
			n.Leafs = append(n.Leafs, node)
		}
	}
}

func (n *Node) isLeafOk(node *Node) bool {
	for _, letter := range node.Bytes {
		if slices.Contains(n.Bytes, letter) {
			return false
		}
	}
	return true
}
