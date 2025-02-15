package csprecon

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/edoardottt/csprecon/pkg/input"
	"github.com/edoardottt/csprecon/pkg/output"
	"github.com/edoardottt/golazy"
	"github.com/projectdiscovery/gologger"
	fileutil "github.com/projectdiscovery/utils/file"
)

type Runner struct {
	Input   chan string
	Output  chan string
	InWg    *sync.WaitGroup
	OutWg   *sync.WaitGroup
	Options input.Options
}

func New(options *input.Options) Runner {
	return Runner{
		Input:   make(chan string, options.Concurrency),
		Output:  make(chan string, options.Concurrency),
		InWg:    &sync.WaitGroup{},
		OutWg:   &sync.WaitGroup{},
		Options: *options,
	}
}

func (r *Runner) Run() {
	r.InWg.Add(1)

	go pushInput(r)
	r.InWg.Add(1)

	go execute(r)
	r.OutWg.Add(1)

	go pullOutput(r)
	r.InWg.Wait()

	close(r.Output)
	r.OutWg.Wait()
}

func pushInput(r *Runner) {
	defer r.InWg.Done()

	if fileutil.HasStdin() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			r.Input <- scanner.Text()
		}
	}

	if r.Options.FileInput != "" {
		for _, line := range golazy.ReadFileLineByLine(r.Options.FileInput) {
			r.Input <- line
		}
	}

	if r.Options.Input != "" {
		r.Input <- r.Options.Input
	}

	close(r.Input)
}

func execute(r *Runner) {
	defer r.InWg.Done()

	regex := regexp.Regexp{}

	if r.Options.Domain != "" {
		regex = *CompileRegex(`.*\.` + r.Options.Domain)
	}

	dregex := CompileRegex(DomainRegex)

	for i := 0; i < r.Options.Concurrency; i++ {
		r.InWg.Add(1)

		go func() {
			defer r.InWg.Done()

			for value := range r.Input {
				client := customClient(r.Options.Timeout)

				result, err := checkCSP(value, dregex, client)
				if err != nil {
					if r.Options.Verbose {
						gologger.Error().Msgf("%s", err)
					}

					return
				}

				for _, res := range result {
					if resTrimmed := strings.TrimSpace(res); resTrimmed != "" {
						if r.Options.Domain != "" {
							if regex.Match([]byte(resTrimmed)) {
								r.Output <- resTrimmed
							}
						} else {
							r.Output <- resTrimmed
						}
					}
				}
			}
		}()
	}
}

func pullOutput(r *Runner) {
	defer r.OutWg.Done()

	out := output.New()

	for o := range r.Output {
		r.OutWg.Add(1)

		go writeOutput(r.OutWg, &r.Options, &out, o)
	}
}

func writeOutput(wg *sync.WaitGroup, options *input.Options, out *output.Result, o string) {
	defer wg.Done()

	if options.FileOutput != "" {
		file, err := os.OpenFile(options.FileOutput, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			gologger.Fatal().Msg(err.Error())
		}

		options.Output = file
	}

	if !out.Printed(o) {
		if options.Output != nil {
			if _, err := options.Output.Write([]byte(o + "\n")); err != nil && options.Verbose {
				gologger.Fatal().Msg(err.Error())
			}
		}

		fmt.Println(o)
	}
}
