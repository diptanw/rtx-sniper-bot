package nvidia

type (
	Product string
)

var (
	ProductRTX5090 = Product("RTX 5090 FE")
	ProductRTX5080 = Product("RTX 5080 FE")
)

func (p Product) SKU(c Country) string {
	switch c {
	case CountrySweden:
		switch p {
		case ProductRTX5080:
			return "1147624"
		case ProductRTX5090:
			return ""
		}
	case CountryDenmark:
		return ""
	case CountryNorway:
		return ""
	case CountryFinland:
		return ""
	case CountryGermany:
		switch p {
		case ProductRTX5080:
			return "307545" // 1145548
		case ProductRTX5090:
			return ""
		}
	case CountryNetherlands:
		return ""
	}

	return ""
}

func (p Product) String() string {
	return string(p)
}
