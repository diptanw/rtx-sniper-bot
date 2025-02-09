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
			return "1147625"
		}
	case CountryDenmark:
		switch p {
		case ProductRTX5080:
			return "1145786"
		case ProductRTX5090:
			return "1145785"
		}
	case CountryFinland:
		switch p {
		case ProductRTX5080:
			return "1147557"
		case ProductRTX5090:
			return "1147616"
		}
	case CountryGermany:
		switch p {
		case ProductRTX5080:
			return "1145548"
		case ProductRTX5090:
			return "1145543"
		}
	case CountryNetherlands:
		switch p {
		case ProductRTX5080:
			return "1147627"
		case ProductRTX5090:
			return "1147626"
		}
	}

	return ""
}

func (p Product) String() string {
	return string(p)
}
