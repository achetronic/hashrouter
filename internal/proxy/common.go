package proxy

import (
	"hashrouter/internal/globals"
	"net"
	"net/http"
	"regexp"
	"strings"
)

const (
	RequestHeaderPattern  = `\$\{REQUEST_HEADER:([^\}]+)\}`
	ResponseHeaderPattern = `\$\{RESPONSE_HEADER:([^\}]+)\}`
	RequestPartsPattern   = `\$\{REQUEST:([^\}]+)\}`
)

var (
	//
	RequestHeadersPatternCompiled  = regexp.MustCompile(RequestHeaderPattern)
	ResponseHeadersPatternCompiled = regexp.MustCompile(ResponseHeaderPattern)

	//
	RequestPartsPatternCompiled = regexp.MustCompile(RequestPartsPattern)
)

// BuildLogFields creates a list of log fields based on the configuration
func BuildLogFields(req *http.Request, res *http.Response, configurationFields []string) []interface{} {
	var logFields []interface{}

	for _, field := range configurationFields {

		result := ReplaceRequestTagsString(req, field)

		result = ReplaceHeaderTagsString(req, res, result)

		// Ignore not expanded fields
		if result == field {
			continue
		}

		// Clean the field name a bit and add it to the fields pool
		field = strings.TrimPrefix(field, "${REQUEST:")
		field = strings.TrimPrefix(field, "${REQUEST_HEADER:")
		field = strings.TrimPrefix(field, "${RESPONSE_HEADER:")
		field = strings.TrimSuffix(field, "}")

		logFields = append(logFields, field, result)
	}

	return logFields
}

// ReplaceRequestTagsString TODO
func ReplaceRequestTagsString(req *http.Request, textToProcess string) (result string) {

	// Replace request parts in the format ${REQUEST:part}
	requestTags := map[string]string{
		"scheme": req.URL.Scheme,
		"host":   req.Host,
		"port":   req.URL.Port(),
		"path":   req.URL.Path,
		"query":  req.URL.Query().Encode(),
		"method": req.Method,
		"proto":  req.Proto,
	}

	result = RequestPartsPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {
		variable := RequestPartsPatternCompiled.FindStringSubmatch(match)[1]

		if replacement, exists := requestTags[variable]; exists {
			return replacement
		}
		return match
	})

	return result
}

// ReplaceHeaderTagsString TODO
func ReplaceHeaderTagsString(req *http.Request, res *http.Response, textToProcess string) (result string) {

	// Replace headers in the format ${REQHEADER:header-name}
	result = RequestHeadersPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {

		variable := RequestHeadersPatternCompiled.FindStringSubmatch(match)[1]
		headerValue := req.Header.Get(globals.CapitalizeWords(variable))

		if headerValue != "" {
			return headerValue
		}

		return match
	})

	// Replace headers in the format ${RESHEADER:header-name}
	result = ResponseHeadersPatternCompiled.ReplaceAllStringFunc(result, func(match string) string {

		variable := ResponseHeadersPatternCompiled.FindStringSubmatch(match)[1]
		headerValue := res.Header.Get(globals.CapitalizeWords(variable))

		if headerValue != "" {
			return headerValue
		}

		return match
	})

	return result
}

// IsIPv6 TODO
func IsIPv6(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To16() != nil && parsedIP.To4() == nil
}
