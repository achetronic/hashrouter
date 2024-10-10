package proxy

import (
	"hashrouter/internal/utils"
	"net"
	"net/http"
	"regexp"
	"strings"
	//
)

const (
	RequestPartsPattern   = `\$\{REQUEST:([^\}]+)\}`
	RequestHeaderPattern  = `\$\{REQUEST_HEADER:([^\}]+)\}`
	ResponseHeaderPattern = `\$\{RESPONSE_HEADER:([^\}]+)\}`
	ExtraPattern          = `\$\{EXTRA:([^\}]+)\}`
)

var (
	//
	RequestPartsPatternCompiled    = regexp.MustCompile(RequestPartsPattern)
	RequestHeadersPatternCompiled  = regexp.MustCompile(RequestHeaderPattern)
	ResponseHeadersPatternCompiled = regexp.MustCompile(ResponseHeaderPattern)
	ExtraPatternCompiled           = regexp.MustCompile(ExtraPattern)
)

// ConnectionExtraData represents internally autogenerated extra data for a connection.
// It is used to replace the 'EXTRA' tags in the log message configuration
type ConnectionExtraData struct {
	RequestId string
	Hashkey   string
	Backend   string
}

// ReplaceRequestTags replaces the HTTP request tags in the given text
// Tags are expressed as ${REQUEST:<part>}, where <part> can be one of the following:
// scheme, host, port, path, query, method, proto
func ReplaceRequestTags(req *http.Request, textToProcess string) (result string) {

	// Replace request parts in the format ${REQUEST:<part>}
	requestTags := map[string]string{
		"scheme": req.URL.Scheme,
		"host":   req.Host,
		"port":   req.URL.Port(),
		"path":   req.URL.Path,
		"query":  req.URL.RawQuery,
		"method": req.Method,
		"proto":  req.Proto,
	}

	result = RequestPartsPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {

		variable := RequestPartsPatternCompiled.FindStringSubmatch(match)[1]

		if replacement, exists := requestTags[variable]; exists {
			return replacement
		}

		return ""
	})

	return result
}

// ReplaceRequestHeaderTags replaces the HTTP request headers in the given text
// Tags are expressed as ${REQUEST_HEADER:<header-name>}
func ReplaceRequestHeaderTags(req *http.Request, textToProcess string) (result string) {

	result = RequestHeadersPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {

		variable := RequestHeadersPatternCompiled.FindStringSubmatch(match)[1]
		headerValue := req.Header.Get(utils.CapitalizeWords(variable))

		return headerValue
	})

	return result
}

// ReplaceResponseHeaderTags replaces the HTTP response headers in the given text
// Tags are expressed as ${RESPONSE_HEADER:<header-name>}
func ReplaceResponseHeaderTags(res *http.Response, textToProcess string) (result string) {

	result = ResponseHeadersPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {

		variable := ResponseHeadersPatternCompiled.FindStringSubmatch(match)[1]
		headerValue := res.Header.Get(utils.CapitalizeWords(variable))

		return headerValue
	})

	return result
}

// ReplaceExtraTags replaces the 'EXTRA' tags in the given text
// Tags are expressed as ${EXTRA:<field-name>}
// where <field-name> is one of the following: request-id, hashkey, backend
func ReplaceExtraTags(extra ConnectionExtraData, textToProcess string) (result string) {

	result = ExtraPatternCompiled.ReplaceAllStringFunc(textToProcess, func(match string) string {

		variable := ExtraPatternCompiled.FindStringSubmatch(match)[1]

		switch variable {
		case "request-id":
			return extra.RequestId
		case "hashkey":
			return extra.Hashkey
		case "backend":
			return extra.Backend
		default:
			return ""
		}
	})

	return result
}

// GetRequestLogFields returns the fields attached to a log message for the given HTTP request
func GetRequestLogFields(req *http.Request, extraData ConnectionExtraData, configurationFields []string) []interface{} {
	var logFields []interface{}

	for _, field := range configurationFields {

		result := ReplaceRequestTags(req, field)

		result = ReplaceRequestHeaderTags(req, result)

		result = ReplaceExtraTags(extraData, result)

		// Ignore not expanded fields
		if result == field {
			continue
		}

		// Clean the field name a bit and add it to the fields pool
		field = strings.TrimPrefix(field, "${REQUEST:")
		field = strings.TrimPrefix(field, "${REQUEST_HEADER:")
		field = strings.TrimPrefix(field, "${EXTRA:")
		field = strings.TrimSuffix(field, "}")

		logFields = append(logFields, field, result)
	}

	return logFields
}

// GetResponseLogFields returns the fields attached to a log message for the given HTTP response
func GetResponseLogFields(res *http.Response, extraData ConnectionExtraData, configurationFields []string) []interface{} {
	var logFields []interface{}

	for _, field := range configurationFields {

		result := ReplaceResponseHeaderTags(res, field)

		result = ReplaceExtraTags(extraData, result)

		// Ignore not expanded fields
		if result == field {
			continue
		}

		// Clean the field name a bit and add it to the fields pool
		field = strings.TrimPrefix(field, "${RESPONSE_HEADER:")
		field = strings.TrimPrefix(field, "${EXTRA:")
		field = strings.TrimSuffix(field, "}")

		logFields = append(logFields, field, result)
	}

	logFields = append(logFields, "status", res.StatusCode)

	return logFields
}

// IsIPv6 checks if the given IP address is an IPv6 address
func IsIPv6(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To16() != nil && parsedIP.To4() == nil
}
