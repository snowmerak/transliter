package transliter

import "fmt"

// Language is a canonical full language name known by at least one model
// integration in this module.
//
// Hy-MT2's model card recommends full language names rather than abbreviations
// in source and target language instructions. Model implementations remain
// responsible for checking their own supported language subset.
type Language string

const (
	LanguageChinese            Language = "Chinese"
	LanguageEnglish            Language = "English"
	LanguageFrench             Language = "French"
	LanguagePortuguese         Language = "Portuguese"
	LanguageSpanish            Language = "Spanish"
	LanguageJapanese           Language = "Japanese"
	LanguageTurkish            Language = "Turkish"
	LanguageRussian            Language = "Russian"
	LanguageArabic             Language = "Arabic"
	LanguageKorean             Language = "Korean"
	LanguageThai               Language = "Thai"
	LanguageItalian            Language = "Italian"
	LanguageGerman             Language = "German"
	LanguageVietnamese         Language = "Vietnamese"
	LanguageMalay              Language = "Malay"
	LanguageIndonesian         Language = "Indonesian"
	LanguageFilipino           Language = "Filipino"
	LanguageHindi              Language = "Hindi"
	LanguageTraditionalChinese Language = "Traditional Chinese"
	LanguagePolish             Language = "Polish"
	LanguageCzech              Language = "Czech"
	LanguageDutch              Language = "Dutch"
	LanguageKhmer              Language = "Khmer"
	LanguageBurmese            Language = "Burmese"
	LanguagePersian            Language = "Persian"
	LanguageGujarati           Language = "Gujarati"
	LanguageUrdu               Language = "Urdu"
	LanguageTelugu             Language = "Telugu"
	LanguageMarathi            Language = "Marathi"
	LanguageHebrew             Language = "Hebrew"
	LanguageBengali            Language = "Bengali"
	LanguageTamil              Language = "Tamil"
	LanguageUkrainian          Language = "Ukrainian"
	LanguageTibetan            Language = "Tibetan"
	LanguageKazakh             Language = "Kazakh"
	LanguageMongolian          Language = "Mongolian"
	LanguageUyghur             Language = "Uyghur"
	LanguageCantonese          Language = "Cantonese"
	LanguageSwedish            Language = "Swedish"
	LanguageNorwegian          Language = "Norwegian"
	LanguageDanish             Language = "Danish"
	LanguageFinnish            Language = "Finnish"
	LanguageIcelandic          Language = "Icelandic"
	LanguageSlovak             Language = "Slovak"
	LanguageHungarian          Language = "Hungarian"
	LanguageRomanian           Language = "Romanian"
	LanguageBulgarian          Language = "Bulgarian"
	LanguageCroatian           Language = "Croatian"
	LanguageSerbian            Language = "Serbian"
	LanguageBosnian            Language = "Bosnian"
	LanguageSlovenian          Language = "Slovenian"
	LanguageGreek              Language = "Greek"
	LanguagePunjabi            Language = "Punjabi"
	LanguageKannada            Language = "Kannada"
	LanguageMalayalam          Language = "Malayalam"
	LanguageSinhala            Language = "Sinhala"
	LanguageNepali             Language = "Nepali"
	LanguageSwahili            Language = "Swahili"
	LanguageAmharic            Language = "Amharic"
	LanguageYoruba             Language = "Yoruba"
	LanguageHausa              Language = "Hausa"
)

var supportedLanguages = [...]Language{
	LanguageChinese,
	LanguageEnglish,
	LanguageFrench,
	LanguagePortuguese,
	LanguageSpanish,
	LanguageJapanese,
	LanguageTurkish,
	LanguageRussian,
	LanguageArabic,
	LanguageKorean,
	LanguageThai,
	LanguageItalian,
	LanguageGerman,
	LanguageVietnamese,
	LanguageMalay,
	LanguageIndonesian,
	LanguageFilipino,
	LanguageHindi,
	LanguageTraditionalChinese,
	LanguagePolish,
	LanguageCzech,
	LanguageDutch,
	LanguageKhmer,
	LanguageBurmese,
	LanguagePersian,
	LanguageGujarati,
	LanguageUrdu,
	LanguageTelugu,
	LanguageMarathi,
	LanguageHebrew,
	LanguageBengali,
	LanguageTamil,
	LanguageUkrainian,
	LanguageTibetan,
	LanguageKazakh,
	LanguageMongolian,
	LanguageUyghur,
	LanguageCantonese,
	LanguageSwedish,
	LanguageNorwegian,
	LanguageDanish,
	LanguageFinnish,
	LanguageIcelandic,
	LanguageSlovak,
	LanguageHungarian,
	LanguageRomanian,
	LanguageBulgarian,
	LanguageCroatian,
	LanguageSerbian,
	LanguageBosnian,
	LanguageSlovenian,
	LanguageGreek,
	LanguagePunjabi,
	LanguageKannada,
	LanguageMalayalam,
	LanguageSinhala,
	LanguageNepali,
	LanguageSwahili,
	LanguageAmharic,
	LanguageYoruba,
	LanguageHausa,
}

var supportedLanguageSet = func() map[Language]struct{} {
	set := make(map[Language]struct{}, len(supportedLanguages))
	for _, language := range supportedLanguages {
		set[language] = struct{}{}
	}
	return set
}()

// Valid reports whether language is known by at least one model integration.
func (language Language) Valid() bool {
	_, ok := supportedLanguageSet[language]
	return ok
}

// String returns the full language name used in a prompt.
func (language Language) String() string {
	return string(language)
}

// ParseLanguage validates an external full language name.
func ParseLanguage(value string) (Language, error) {
	language := Language(value)
	if !language.Valid() {
		return "", fmt.Errorf("unsupported language %q", value)
	}
	return language, nil
}

// SupportedLanguages returns a copy of the declared Hy-MT2 language list.
func SupportedLanguages() []Language {
	languages := make([]Language, len(supportedLanguages))
	copy(languages, supportedLanguages[:])
	return languages
}
