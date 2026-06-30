package zipcode

import "unicode"

func Valid(cep string) bool {
	if len(cep) != 8 {
		return false
	}

	for _, char := range cep {
		if !unicode.IsDigit(char) {
			return false
		}
	}

	return true
}
