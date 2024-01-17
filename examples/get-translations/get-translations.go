package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	"github.com/d0rc/agent-os/utils"
	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var path = flag.String("locale-path", "/tmp/locale_keys", "path to locale keys")
var agentOSUrl = flag.String("agent-os", "http://127.0.0.1:9000", "agent-os endpoint")
var processName = flag.String("process-name", "app-translator", "process name")
var agentOSToken = flag.String("agent-os-auth", "auth-token", "agent-os auth token")
var aiMotto = flag.String("ai-motto", "You're Application Resources Translation AI. ", "what translator needs to think of himself")

var counter = uint64(0)

func main() {
	ts := time.Now()
	lg, _ := utils.ConsoleInit(*processName, nil)

	client := os_client.NewAgentOSClient(*agentOSUrl)

	lg.Info().Msgf("starting up with path: %s", *path)
	appDirs, err := os.ReadDir(*path)
	if err != nil {
		lg.Fatal().Err(err).Msgf("failed to read path: %s", *path)
	}

	existingTranslations := readAppsResources(appDirs, lg)

	existingAppLocalesN := 0
	existingLangsMap := make(map[string]struct{})
	for _, app := range existingTranslations {
		for localeName, _ := range *app {
			existingAppLocalesN++
			existingLangsMap[localeName] = struct{}{}
		}
	}

	lg.Info().Msgf("found %d titles and %d locales in %d languages.",
		len(existingTranslations),
		existingAppLocalesN,
		len(existingLangsMap))

	targetLocale := "cs_CZ"
	//targetLocale := "uk_UA"
	wg := sync.WaitGroup{}
	maxThreads := make(chan struct{}, 96)
	for appName, app := range existingTranslations {
		appData := (*app)
		if _, exists := appData[targetLocale]; !exists {
			wg.Add(1)
			maxThreads <- struct{}{}
			go func(client *os_client.AgentOSClient, lg zerolog.Logger, name AppTitle, locale string, translations map[AppTitle]*AppLocales) {
				lg.Info().Msgf("locale %s not found in %s", targetLocale, appName)
				createTranslations(client, lg, appName, targetLocale, existingTranslations)
				<-maxThreads
				wg.Done()
			}(client, lg, appName, targetLocale, existingTranslations)
		}
	}

	wg.Wait()

	fmt.Printf("\nDone translating %d terms in %v\n",
		counter,
		time.Since(ts))
}

func readAppsResources(appDirs []os.DirEntry, lg zerolog.Logger) map[AppTitle]*AppLocales {
	existingTranslations := make(map[AppTitle]*AppLocales)
	for _, appDir := range appDirs {
		if appDir.IsDir() {
			lg.Info().Msgf("loading app title: %s", appDir.Name())

			appTitle := AppTitle(appDir.Name())

			appLocales, err := os.ReadDir(fmt.Sprintf("%s/%s", *path, appTitle))
			if err != nil {
				lg.Fatal().Err(err).Msgf("failed to read path: %s", fmt.Sprintf("%s/%s", *path, appDir.Name()))
			}

			collectedLocales := make(AppLocales)
			for _, appLocale := range appLocales {
				if strings.HasSuffix(appLocale.Name(), ".json") {
					jsonBytes, err := os.ReadFile(fmt.Sprintf("%s/%s/%s", *path, appTitle, appLocale.Name()))
					if err != nil {
						lg.Fatal().Err(err).Msgf("failed to read path: %s", fmt.Sprintf("%s/%s/%s", *path, appTitle, appLocale.Name()))
					}

					appTerms := make(LocaleTerms)
					err = json.Unmarshal(jsonBytes, &appTerms)
					if err != nil {
						lg.Fatal().Err(err).Msgf("failed to unmarshal json: %s", jsonBytes)
					}

					collectedLocales[strings.TrimSuffix(appLocale.Name(), ".json")] = &appTerms
				}
			}

			existingTranslations[appTitle] = &collectedLocales
		}
	}
	return existingTranslations
}

func getGroundTruthFromOtherApps(translations map[AppTitle]*AppLocales, term, locale string) ([]string, error) {
	// scan all translations to find all possible translations for this exact term
	var groundTruth []string
	for _, appLocales := range translations {
		if _, exists := (*appLocales)[locale]; !exists {
			continue
		}
		appTerms := *(*appLocales)[locale]
		if termValue, exists := appTerms[term]; !exists {
			continue
		} else {
			data, ok := termValue.(string)
			if ok && data != "" {
				groundTruth = append(groundTruth, termValue.(string))
			}
		}
	}
	return tools.DropDuplicates(groundTruth), nil
}

func createTranslations(client *os_client.AgentOSClient, lg zerolog.Logger, name AppTitle, locale string, translations map[AppTitle]*AppLocales) {
	appTerms := make(map[string]map[string]string)
	appData := translations[name]
	for localeName, localeTerms := range *appData {
		termData := *localeTerms
		for termName, termTranslation := range termData {
			termTranslationString := ""
			switch termTranslation.(type) {
			case string:
				termTranslationString = termTranslation.(string)
			case int:
				termTranslationString = fmt.Sprintf("%d", termTranslation.(int))
			case float64:
				termTranslationString = fmt.Sprintf("%d", int(termTranslation.(float64)))
			default:
				lg.Fatal().Msgf("failed to guess type of %v", termTranslation)
			}

			if _, exists := appTerms[termName]; !exists {
				appTerms[termName] = make(map[string]string)
			}
			appTerms[termName][localeName] = termTranslationString
		}
	}

	targetLocaleData := make(LocaleTerms)
	for termName, termTranslations := range appTerms {
		existingExamples := ""
		for localeName, termTranslation := range termTranslations {
			if existingExamples != "" {
				existingExamples += "\n"
			}
			existingExamples += fmt.Sprintf("%s: \"%s\"", localeName, termTranslation)
		}

		translationPrompt := `### Instruction: %s

You're reviewing this term: %s

It has following translations already selected:
%s

Provide a single translation option for the term %s to %s language. 

### Assistant:
%s: "`
		translationPrompt = fmt.Sprintf(translationPrompt, *aiMotto, termName, existingExamples, termName, locale, locale)

		atomic.AddUint64(&counter, 1)
		res, err := client.RunRequest(&cmds.ClientRequest{
			ProcessName: *processName,
			GetCompletionRequests: []cmds.GetCompletionRequest{
				{
					RawPrompt:   translationPrompt,
					Temperature: 0.5,
					MinResults:  1,
				},
			},
		}, 1*time.Minute, os_client.REP_IO)
		if err != nil {
			lg.Fatal().Err(err).Msgf("failed to run request")
		}

		if len(res.GetCompletionResponse) == 0 {
			lg.Fatal().Msgf("no completions")
		}

		translationCandidates := make([]string, 0)
		for _, choice := range tools.FlattenChoices(res.GetCompletionResponse) {
			translationCandidates = append(translationCandidates, fmt.Sprintf("%s", strings.TrimSuffix(choice, "\"")))
		}

		gt, _ := getGroundTruthFromOtherApps(translations, termName, locale)

		fmt.Printf("[%s:%6d]:[%s]:[%10s] [%20s] => [%v]; gt = [%v]\n",
			aurora.BrightYellow("RESULT"),
			counter,
			aurora.BrightGreen(locale),
			aurora.BrightCyan(name),
			aurora.BrightWhite(termName),
			aurora.White(encode(translationCandidates[0])),
			aurora.White(strings.Join(gt, ",")),
		)
		targetLocaleData[termName] = translationCandidates[0]
	}

	serialized, err := json.Marshal(targetLocaleData)
	if err != nil {
		lg.Fatal().Err(err).Msgf("failed to marshal json")
	}

	err = os.WriteFile(fmt.Sprintf("%s/%s/%s.xson", *path, name, locale), serialized, 0644)
}

func encode(s string) string {
	res, _ := json.Marshal(s)
	return string(res)
}

type AppTitle string
type LocaleTerms map[string]interface{}
type AppLocales map[string]*LocaleTerms
