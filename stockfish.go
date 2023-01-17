package stockfish

// doesnt do much on its own

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/notnil/chess"
)


type matchRequest struct {
	match regexp.Regexp
	channel chan string
	perma bool
}

type StockFish struct {
	process exec.Cmd
	scan bufio.Scanner
	write bufio.Writer
	hasPutIsReady bool
	hasGotIsReady bool
	matchRequests []*matchRequest
	lastScoreWasMate bool
	lastScore int
	hasLastScore bool
	whitesMove bool
}

const scoreRegex = "info depth \\d+ seldepth \\d+ multipv \\d+ score (?:(cp -?\\d+)|(mate -?\\d+)).*"

func (s *StockFish) Init() {
	s.process = *exec.Command("./stockfish")
	var err error
	var stdout io.ReadCloser
	var stdin io.WriteCloser
	if stdout, err = s.process.StdoutPipe(); err != nil {
		panic(err)
	}
	s.scan = *bufio.NewScanner(stdout)
	if stdin, err = s.process.StdinPipe(); err != nil {
		panic(err)
	}
	s.write = *bufio.NewWriter(stdin)
	if err = s.process.Start(); err != nil {
		panic(err)
	}
	go s.Loop()
	ir := s.WaitForRaw(*regexp.MustCompile("readyok"), true)
	infoscore := s.WaitForRaw(*regexp.MustCompile(scoreRegex), true)
	go s.IBChannelWatch(ir, infoscore)
	s.WriteRaw("uci")
	<- s.WaitForRaw(*regexp.MustCompile("uciok"), false)
	s.NewGame()
}

func trimLeftChars(s string, n int) string {
    m := 0
    for i := range s {
        if m >= n {
            return s[i:]
        }
        m++
    }
    return s[:0]
}

func (s *StockFish) IBChannelWatch(isready, score chan string) {
	for {
		select {
		case <- isready:
			s.hasGotIsReady = true
		case match := <- score:
			sm := regexp.MustCompile(scoreRegex).FindStringSubmatch(match)
			if len(sm[1]) > 0 {
				cpEval := trimLeftChars(sm[1], 3)
				s.lastScoreWasMate = false
				a, err := strconv.Atoi(cpEval)
				if err == nil {
					s.lastScore = a
					s.hasLastScore = true
				} else {
					println(err)
				}
			}
			if len(sm[2]) > 0 {
				mateIn := trimLeftChars(sm[2], 5)
				s.lastScoreWasMate = true
				a, err := strconv.Atoi(mateIn)
				if err == nil {
					s.lastScore = a
					s.hasLastScore = true
				} else {
					println(err)
				}
			}
		}
	}
}

func (s *StockFish) NewGame() {
	s.WriteRaw("ucinewgame")
	s.WaitReady()
	s.whitesMove = true
}

func (s *StockFish) Load(game chess.Game) {
	s.WriteRaw("position fen " + game.FEN())
	s.whitesMove = game.Position().Turn() == chess.White
}

func (s *StockFish) Eval(depth int) (int, int, error) { // returns cp eval, mate in x, error
	s.SetOption("Ponder", "value false")
	s.SetOption("UCI_AnalyseMode", "value true")
	s.WriteRaw("go depth " + fmt.Sprint(depth))
	s.hasLastScore = false
	<- s.WaitForRaw(*regexp.MustCompile("bestmove.*"), false)
	if (!s.hasLastScore) {
		return 0, 0, fmt.Errorf("Eval failed")
	}
	if !s.whitesMove {
		s.lastScore = s.lastScore * -1
	}
	if s.lastScoreWasMate {
		return 0, s.lastScore, nil
	} else {
		return s.lastScore, 0, nil
	}
}

func (s *StockFish) SetOption(option string, after string) {
	s.WriteRaw("setoption name " + option + " " + after)
}

func (s *StockFish) Loop() {
	for {
		s.scan.Scan()
		txt := s.scan.Text()
		i := 0
		new := s.matchRequests[:0]
eachMR:
		for _, mr := range s.matchRequests {
			if mr.match.MatchString(txt) {
				mr.channel <- txt
				if !mr.perma {
					close(mr.channel)
					continue eachMR
				}
			}
			new = append(new, mr)
			i++
		}
		//println(txt)
		s.matchRequests = new
	}
}


func (s *StockFish) WriteRaw(cmd string) {
	s.write.Write([]byte(cmd + "\n"))
	s.write.Flush()
}


func (s *StockFish) WaitForRaw(matches regexp.Regexp, perma bool) chan string {
	onFound := make(chan string)
	mr := matchRequest {channel: onFound, match: matches, perma: perma}
	s.matchRequests = append(s.matchRequests, &mr)
	return onFound
}

func (s *StockFish) Close() {
	s.process.Process.Kill()
}

func (s *StockFish) PutIsReady() {
	s.write.Write([]byte("isready\n"))
	s.write.Flush()
	s.hasPutIsReady = true
}

func (s *StockFish) GetIsReady() bool {
	if (s.hasGotIsReady) {
		s.hasGotIsReady = false
		s.hasPutIsReady = false
		return true
	}
	return false
}

func (s *StockFish) WaitReady() {
	s.PutIsReady()
	for !s.GetIsReady() {}
}

