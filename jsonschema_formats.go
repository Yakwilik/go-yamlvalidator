package yamlvalidator

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	uritemplate "github.com/yosida95/uritemplate/v3"
	"golang.org/x/net/idna"
)

var jsonSchemaIDNAProfile = idna.Lookup

func registerExtendedJSONSchemaFormats(compiler *jsonschema.Compiler) {
	// jsonschema/v6 already provides the standard ASCII formats. These two
	// internationalized formats are part of the JSON Schema format vocabulary
	// but are not built into the engine.
	compiler.RegisterFormat(&jsonschema.Format{Name: "email", Validate: validateJSONSchemaEmail})
	compiler.RegisterFormat(&jsonschema.Format{Name: "hostname", Validate: validateJSONSchemaHostname})
	compiler.RegisterFormat(&jsonschema.Format{Name: "ipv4", Validate: validateJSONSchemaIPv4})
	compiler.RegisterFormat(&jsonschema.Format{Name: "duration", Validate: validateJSONSchemaDuration})
	compiler.RegisterFormat(&jsonschema.Format{Name: "uri", Validate: validateJSONSchemaURI})
	compiler.RegisterFormat(&jsonschema.Format{Name: "uri-reference", Validate: validateJSONSchemaURIReference})
	compiler.RegisterFormat(&jsonschema.Format{Name: "uri-template", Validate: validateJSONSchemaURITemplate})
	compiler.RegisterFormat(&jsonschema.Format{Name: "idn-hostname", Validate: validateJSONSchemaIDNHostname})
	compiler.RegisterFormat(&jsonschema.Format{Name: "idn-email", Validate: validateJSONSchemaIDNEmail})
}

func validateJSONSchemaIPv4(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("invalid IPv4 address")
	}
	return nil
}

func validateJSONSchemaURI(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if err := validateRFC3986Reference(s, true); err != nil {
		return err
	}
	return nil
}

func validateJSONSchemaURIReference(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	return validateRFC3986Reference(s, false)
}

func validateJSONSchemaURITemplate(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	_, err := uritemplate.New(s)
	return err
}

func validateRFC3986Reference(value string, requireScheme bool) error {
	if value == "" && requireScheme {
		return fmt.Errorf("URI must contain a scheme")
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch >= 0x80 {
			return fmt.Errorf("URI contains unescaped non-ASCII character")
		}
		if ch <= 0x20 || ch == 0x7f || strings.ContainsRune(`<>\\^`+"`"+`{|}"`, rune(ch)) {
			return fmt.Errorf("URI contains invalid character %q", ch)
		}
		if ch == '%' {
			if i+2 >= len(value) || !isHexByte(value[i+1]) || !isHexByte(value[i+2]) {
				return fmt.Errorf("URI contains invalid percent-encoding")
			}
			i += 2
		}
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if requireScheme && !validURIScheme(parsed.Scheme) {
		return fmt.Errorf("URI must contain a valid scheme")
	}

	// RFC 3986 allows '[' and ']' only around an IP-literal in the authority.
	withoutAuthority := value
	if idx := strings.Index(withoutAuthority, "//"); idx >= 0 {
		authorityStart := idx + 2
		authorityEnd := len(withoutAuthority)
		if end := strings.IndexAny(withoutAuthority[authorityStart:], "/?#"); end >= 0 {
			authorityEnd = authorityStart + end
		}
		authority := withoutAuthority[authorityStart:authorityEnd]
		if strings.Count(authority, "@") > 1 {
			return fmt.Errorf("URI authority contains more than one raw @ delimiter")
		}
		outside := withoutAuthority[:authorityStart] + withoutAuthority[authorityEnd:]
		if strings.ContainsAny(outside, "[]") {
			return fmt.Errorf("URI contains square brackets outside authority")
		}
	} else if strings.ContainsAny(withoutAuthority, "[]") {
		return fmt.Errorf("URI contains square brackets outside authority")
	}
	return nil
}

func validURIScheme(scheme string) bool {
	if scheme == "" || !isASCIIAlpha(scheme[0]) {
		return false
	}
	for i := 1; i < len(scheme); i++ {
		ch := scheme[i]
		if isASCIIAlpha(ch) || ch >= '0' && ch <= '9' || ch == '+' || ch == '-' || ch == '.' {
			continue
		}
		return false
	}
	return true
}

func isASCIIAlpha(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

func isHexByte(ch byte) bool {
	return ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F'
}

var (
	jsonSchemaDurationDate = regexp.MustCompile(`^(?:[0-9]+Y(?:[0-9]+M(?:[0-9]+D)?)?|[0-9]+M(?:[0-9]+D)?|[0-9]+D)$`)
	jsonSchemaDurationTime = regexp.MustCompile(`^(?:[0-9]+H(?:[0-9]+M(?:[0-9]+S)?)?|[0-9]+M(?:[0-9]+S)?|[0-9]+S)$`)
	jsonSchemaDurationWeek = regexp.MustCompile(`^[0-9]+W$`)
)

func validateJSONSchemaDuration(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if len(s) < 2 || s[0] != 'P' {
		return fmt.Errorf("invalid RFC 3339 duration")
	}
	body := s[1:]
	if jsonSchemaDurationWeek.MatchString(body) {
		return nil
	}
	if strings.Count(body, "T") > 1 {
		return fmt.Errorf("invalid RFC 3339 duration")
	}
	datePart, timePart, hasTime := strings.Cut(body, "T")
	if hasTime {
		if timePart == "" || !jsonSchemaDurationTime.MatchString(timePart) {
			return fmt.Errorf("invalid RFC 3339 duration time component")
		}
		if datePart == "" {
			return nil
		}
	}
	if datePart == "" || !jsonSchemaDurationDate.MatchString(datePart) {
		return fmt.Errorf("invalid RFC 3339 duration date component")
	}
	return nil
}

func validateJSONSchemaHostname(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if s == "" || len(s) > 253 || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return fmt.Errorf("invalid hostname")
	}
	for _, r := range s {
		if r > 127 {
			return fmt.Errorf("hostname must contain ASCII characters only")
		}
	}
	for _, label := range strings.Split(s, ".") {
		if err := validateJSONSchemaASCIIHostnameLabel(label); err != nil {
			return err
		}
	}
	return nil
}

func validateJSONSchemaASCIIHostnameLabel(label string) error {
	if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return fmt.Errorf("invalid hostname label")
	}
	for _, ch := range label {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
			continue
		}
		return fmt.Errorf("invalid hostname character %q", ch)
	}
	if strings.HasPrefix(strings.ToLower(label), "xn--") {
		unicodeLabel, err := jsonSchemaIDNAProfile.ToUnicode(label)
		if err != nil || unicodeLabel == label || isASCIIString(unicodeLabel) {
			return fmt.Errorf("invalid IDNA A-label")
		}
		if err := validateJSONSchemaIDNALabelContext(unicodeLabel); err != nil {
			return err
		}
		asciiLabel, err := jsonSchemaIDNAProfile.ToASCII(unicodeLabel)
		if err != nil || !strings.EqualFold(asciiLabel, label) {
			return fmt.Errorf("invalid IDNA A-label")
		}
	}
	return nil
}

func validateJSONSchemaEmail(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	local, domain, ok := splitJSONSchemaMailbox(s)
	if !ok || local == "" || domain == "" || len([]byte(local)) > 64 {
		return fmt.Errorf("invalid email")
	}
	if err := validateJSONSchemaASCIILocalPart(local); err != nil {
		return err
	}
	if strings.HasPrefix(domain, "[") {
		return validateJSONSchemaAddressLiteral(domain)
	}
	return validateJSONSchemaHostname(domain)
}

func validateJSONSchemaASCIILocalPart(local string) error {
	if strings.HasPrefix(local, `"`) {
		if len(local) < 2 || local[len(local)-1] != '"' {
			return fmt.Errorf("invalid quoted local part")
		}
		for i := 1; i < len(local)-1; i++ {
			ch := local[i]
			if ch >= 128 {
				return fmt.Errorf("non-ASCII character in email local part")
			}
			if ch == '\\' {
				i++
				if i >= len(local)-1 || local[i] < 32 || local[i] > 126 {
					return fmt.Errorf("invalid quoted pair in email local part")
				}
				continue
			}
			if ch == '"' || ch < 32 || ch > 126 {
				return fmt.Errorf("invalid quoted email local part")
			}
		}
		return nil
	}
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return fmt.Errorf("invalid email local part dots")
	}
	for i := 0; i < len(local); i++ {
		if local[i] >= 128 || !isJSONSchemaEmailAText(local[i]) {
			return fmt.Errorf("invalid email local part character")
		}
	}
	return nil
}

func validateJSONSchemaAddressLiteral(domain string) error {
	if len(domain) < 3 || domain[0] != '[' || domain[len(domain)-1] != ']' {
		return fmt.Errorf("invalid email address literal")
	}
	literal := domain[1 : len(domain)-1]
	if len(literal) >= 5 && strings.EqualFold(literal[:5], "IPv6:") {
		addr, err := netip.ParseAddr(literal[5:])
		if err != nil || !addr.Is6() {
			return fmt.Errorf("invalid IPv6 email address literal")
		}
		return nil
	}
	addr, err := netip.ParseAddr(literal)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("invalid IPv4 email address literal")
	}
	return nil
}

func isASCIIString(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}

func validateJSONSchemaIDNHostname(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	if s == "" || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return fmt.Errorf("invalid internationalized hostname")
	}
	if isASCIIString(s) {
		for _, label := range strings.Split(s, ".") {
			if err := validateJSONSchemaASCIIHostnameLabel(label); err != nil {
				return err
			}
		}
	}
	ascii, err := jsonSchemaIDNAProfile.ToASCII(s)
	if err != nil {
		return err
	}
	unicodeDomain, err := jsonSchemaIDNAProfile.ToUnicode(s)
	if err != nil {
		return err
	}
	for _, label := range strings.Split(unicodeDomain, ".") {
		if err := validateJSONSchemaIDNALabelContext(label); err != nil {
			return err
		}
	}
	if ascii == "" || len(ascii) > 253 {
		return fmt.Errorf("invalid internationalized hostname length")
	}
	for _, label := range strings.Split(ascii, ".") {
		if err := validateJSONSchemaASCIIHostnameLabel(label); err != nil {
			return err
		}
	}
	return nil
}

func validateJSONSchemaIDNALabelContext(label string) error {
	runes := []rune(label)
	for i, r := range runes {
		switch r {
		case '\u00a1', '\u302e', '\u302f', '\u0640', '\u07fa', '\u3031', '\u3032', '\u3033', '\u3034', '\u3035', '\u303b':
			return fmt.Errorf("disallowed IDNA code point U+%04X", r)
		case '\u00b7': // MIDDLE DOT: only between two ASCII 'l' characters.
			if i == 0 || i+1 >= len(runes) || runes[i-1] != 'l' || runes[i+1] != 'l' {
				return fmt.Errorf("invalid IDNA middle-dot context")
			}
		case '\u0375': // GREEK LOWER NUMERAL SIGN: must be followed by Greek.
			if i+1 >= len(runes) || !unicode.In(runes[i+1], unicode.Greek) {
				return fmt.Errorf("invalid IDNA Greek keraia context")
			}
		case '\u05f3', '\u05f4': // Hebrew punctuation: must be preceded by Hebrew.
			if i == 0 || !unicode.In(runes[i-1], unicode.Hebrew) {
				return fmt.Errorf("invalid IDNA Hebrew punctuation context")
			}
		case '\u30fb': // KATAKANA MIDDLE DOT requires Hiragana, Katakana, or Han in label.
			hasJapaneseScript := false
			for _, other := range runes {
				if unicode.In(other, unicode.Hiragana, unicode.Katakana, unicode.Han) {
					hasJapaneseScript = true
					break
				}
			}
			if !hasJapaneseScript {
				return fmt.Errorf("invalid IDNA Katakana middle-dot context")
			}
		}
	}
	return nil
}

func validateJSONSchemaIDNEmail(value any) error {
	s, ok := value.(string)
	if !ok {
		return nil
	}
	local, domain, ok := splitJSONSchemaMailbox(s)
	if !ok || local == "" || domain == "" || len([]byte(local)) > 64 {
		return fmt.Errorf("invalid internationalized email")
	}
	if err := validateJSONSchemaInternationalLocalPart(local); err != nil {
		return err
	}
	return validateJSONSchemaIDNHostname(domain)
}

func splitJSONSchemaMailbox(value string) (local, domain string, ok bool) {
	if strings.HasPrefix(value, `"`) {
		escaped := false
		for i := 1; i < len(value); i++ {
			switch {
			case escaped:
				escaped = false
			case value[i] == '\\':
				escaped = true
			case value[i] == '"':
				if i+1 >= len(value) || value[i+1] != '@' {
					return "", "", false
				}
				return value[:i+1], value[i+2:], true
			}
		}
		return "", "", false
	}
	at := strings.LastIndexByte(value, '@')
	if at <= 0 || at == len(value)-1 || strings.Contains(value[:at], "@") {
		return "", "", false
	}
	return value[:at], value[at+1:], true
}

func validateJSONSchemaInternationalLocalPart(local string) error {
	if strings.HasPrefix(local, `"`) {
		if len(local) < 2 || !strings.HasSuffix(local, `"`) {
			return fmt.Errorf("invalid quoted local part")
		}
		return nil
	}
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return fmt.Errorf("invalid local part dots")
	}
	for len(local) > 0 {
		r, size := utf8.DecodeRuneInString(local)
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("invalid UTF-8 in local part")
		}
		if r < utf8.RuneSelf && !isJSONSchemaEmailAText(byte(r)) {
			return fmt.Errorf("invalid local part character %q", r)
		}
		local = local[size:]
	}
	return nil
}

func isJSONSchemaEmailAText(ch byte) bool {
	if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' {
		return true
	}
	return strings.ContainsRune("!#$%&'*+-/=?^_`{|}~.", rune(ch))
}
