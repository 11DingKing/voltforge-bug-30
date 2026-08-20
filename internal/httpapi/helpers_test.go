package httpapi

import "strings"

func stringReader(s string) *strings.Reader {
	return strings.NewReader(s)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	if i < 0 {
		buf = append(buf, '-')
		i = -i
	}
	var digits []byte
	for i > 0 {
		digits = append(digits, byte('0'+i%10))
		i /= 10
	}
	for j := len(digits) - 1; j >= 0; j-- {
		buf = append(buf, digits[j])
	}
	if len(buf) == 0 {
		return "0"
	}
	return string(buf)
}
