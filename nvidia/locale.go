package nvidia

type Country string

var (
	CountrySweden      = Country("Sweden")
	CountryDenmark     = Country("Denmark")
	CountryFinland     = Country("Finland")
	CountryGermany     = Country("Germany")
	CountryNetherlands = Country("Netherlands")
)

func (c Country) Locale() string {
	switch c {
	case CountrySweden:
		return "se"
	case CountryDenmark:
		return "dk"
	case CountryFinland:
		return "fi"
	case CountryGermany:
		return "de"
	case CountryNetherlands:
		return "nl"
	default:
		return ""
	}
}

func (c Country) String() string {
	return string(c)
}
