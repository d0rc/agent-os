package utils

import "github.com/wbrown/gpt_bpe"

func TokenizeGPT2(s string) ([]interface{}, error) {
	tokenizer := gpt_bpe.NewGPT2Encoder()
	tokens := tokenizer.Encode(&s)
	resultingTokens := make([]interface{}, 0, len(*tokens))
	for _, token := range *tokens {
		resultingTokens = append(resultingTokens, token)
	}

	return resultingTokens, nil
}

func TokensToStringGPT2(iTokens []interface{}) string {
	tokens := make([]gpt_bpe.Token, 0, len(iTokens))
	for _, iTok := range iTokens {
		tokens = append(tokens, iTok.(gpt_bpe.Token))
	}
	tokenizer := gpt_bpe.NewGPT2Encoder()
	convertedTokens := gpt_bpe.Tokens(tokens)
	recoveredString := tokenizer.Decode(&convertedTokens)

	return recoveredString
}
