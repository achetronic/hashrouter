package proxy

import (
	"fmt"
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

		result, _ := ReplaceRequestTagsString(req, field)

		result, _ = ReplaceHeaderTagsString(req, res, result)

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

// ReplaceRequestTagsString replaces the HTTP request tags in the given text
// Tags are expressed as ${REQUEST:<part>}, where <part> can be one of the following:
// scheme, host, port, path, query, method, proto
func ReplaceRequestTagsString(req *http.Request, textToProcess string) (result string, err error) {

	reqPathParts := strings.Split(req.URL.Path, "?")

	//
	reqQuery := ""
	if len(reqPathParts) > 1 {
		reqQuery = reqPathParts[1]
	}

	// Replace request parts in the format ${REQUEST:<part>}
	requestTags := map[string]string{
		"scheme": req.URL.Scheme,
		"host":   req.Host,
		"port":   req.URL.Port(),
		"path":   reqPathParts[0],
		"query":  reqQuery,
		"method": req.Method,
		"proto":  req.Proto,
	}

	unknownReplacements := make([]string, 0)

	result = RequestPartsPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {

		variable := RequestPartsPatternCompiled.FindStringSubmatch(match)[1]

		if replacement, exists := requestTags[variable]; exists {
			return replacement
		}

		// As ReplaceAllStringFunc does not support returning an error,
		// we need to store it for later checks
		unknownReplacements = append(unknownReplacements, match)
		return match
	})

	if len(unknownReplacements) > 0 {
		strings.Join(unknownReplacements, ", ")
		err = fmt.Errorf("errors while replacing HTTP request parts '%s' in pattern", strings.Join(unknownReplacements, ", "))
	}

	return result, err
}

// ReplaceHeaderTagsString replaces the HTTP request and response headers in the given text
// Tags are expressed as ${REQUEST_HEADER:<header-name>} and ${RESPONSE_HEADER:<header-name>}
// where <header-name> is the name of the header to replace
func ReplaceHeaderTagsString(req *http.Request, res *http.Response, textToProcess string) (result string, err error) {

	unknownReplacements := make([]string, 0)

	// Replace headers in the format ${REQUEST_HEADER:<header-name>}
	result = RequestHeadersPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {

		variable := RequestHeadersPatternCompiled.FindStringSubmatch(match)[1]
		headerValue := req.Header.Get(globals.CapitalizeWords(variable))

		if headerValue != "" {
			return headerValue
		}

		// As ReplaceAllStringFunc does not support returning an error,
		// we need to store it for later checks
		unknownReplacements = append(unknownReplacements, match)
		return match
	})

	// Replace headers in the format ${RESPONSE_HEADER:<header-name>}
	result = ResponseHeadersPatternCompiled.ReplaceAllStringFunc(result, func(match string) string {

		variable := ResponseHeadersPatternCompiled.FindStringSubmatch(match)[1]
		headerValue := res.Header.Get(globals.CapitalizeWords(variable))

		if headerValue != "" {
			return headerValue
		}

		// As ReplaceAllStringFunc does not support returning an error,
		// we need to store it for later checks
		unknownReplacements = append(unknownReplacements, match)
		return match
	})

	if len(unknownReplacements) > 0 {
		strings.Join(unknownReplacements, ", ")
		err = fmt.Errorf("errors while replacing HTTP headers '%s' in pattern", strings.Join(unknownReplacements, ", "))
	}
	return result, err
}

// IsIPv6 checks if the given IP address is an IPv6 address
func IsIPv6(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To16() != nil && parsedIP.To4() == nil
}
