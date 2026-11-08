package harvest

import "fmt"

// MaxGoogleNewsQueries bounds the complete rotation, independently of the per-scan budget.
const MaxGoogleNewsQueries = 400

// googleNewsLanguageQueries pairs cannabis terms with enforcement actions in each
// language. Editions are checked against live RSS responses: unsupported languages
// can silently redirect to another edition. English fallbacks are explicit and
// best effort; an edition is a search locale, not a country filter.
var googleNewsLanguageQueries = []GoogleNewsQuery{
	{"US:en", DefaultGoogleNewsQuery},
	// South Asia: English and supported regional-language editions.
	{"IN:en", `(ganja OR charas OR bhang OR cannabis OR marijuana OR hashish) (seized OR recovered OR arrested OR NDPS OR uprooted OR destroyed OR convicted OR sentenced OR jailed) when:1d`},
	{"IN:hi", `(गांजा OR गांजे OR गाँजा OR चरस OR भांग) (जब्त OR बरामद OR गिरफ्तार OR तस्कर OR नष्ट OR खेती OR सजा OR कारावास) when:1d`},
	{"IN:bn", `(গাঁজা OR গাঁজাসহ OR চরস OR হাশিশ) (উদ্ধার OR জব্দ OR গ্রেপ্তার OR গ্রেফতার OR আটক OR ধ্বংস OR দণ্ড OR কারাদণ্ড) when:1d`},
	{"IN:mr", `(गांजा OR गांजाची OR चरस OR भांग) (जप्त OR अटक OR तस्कर OR नष्ट OR शेती OR शिक्षा) when:1d`},
	{"IN:ta", `(கஞ்சா OR கஞ்சாவை) (பறிமுதல் OR கைது OR கடத்தல் OR அழிப்பு OR தண்டனை OR சிறை) when:1d`},
	{"IN:te", `(గంజాయి OR గంజాయిని) (స్వాధీనం OR అరెస్ట్ OR అరెస్టు OR పట్టివేత OR స్మగ్లింగ్ OR ధ్వంసం OR శిక్ష) when:1d`},
	{"IN:ml", `(കഞ്ചാവ് OR കഞ്ചാവുമായി OR ഹാഷിഷ്) (പിടികൂടി OR പിടിയിൽ OR അറസ്റ്റ് OR പിടിച്ചെടുത്തു OR നശിപ്പിച്ചു OR ശിക്ഷ OR തടവ്) when:1d`},
	{"IN:gu", `(ગાંજો OR ગાંજા OR ચરસ OR ભાંગ) (ઝડપાયો OR ઝડપાયા OR કબજે OR જપ્ત OR ધરપકડ OR સજા) when:1d`},
	{"IN:pa", `(ਗਾਂਜਾ OR ਗਾਂਜੇ OR ਚਰਸ OR ਭੰਗ) (ਬਰਾਮਦ OR ਗ੍ਰਿਫਤਾਰ OR ਜ਼ਬਤ OR ਤਸਕਰ OR ਸਜ਼ਾ) when:1d`},
	{"IN:en", `(Nepal OR "Sri Lanka" OR Bangladesh OR Bhutan) (ganja OR cannabis OR hashish OR marijuana) (seized OR recovered OR arrested OR uprooted OR destroyed OR sentenced) when:1d`},
	{"IN:hi", `(गाँजा OR गांजा OR चरस OR चरेस) (बरामद OR पक्राउ OR नष्ट OR गिरफ्तार OR कैद) when:1d`},
	{"BD:bn", `(গাঁজা OR গাঁজাসহ OR চরস OR হাশিশ) (উদ্ধার OR জব্দ OR গ্রেপ্তার OR গ্রেফতার OR আটক OR ধ্বংস OR দণ্ড OR কারাদণ্ড) when:1d`},
	{"PK:en", `(charas OR bhang OR cannabis OR marijuana OR hashish) (seized OR recovered OR arrested OR ANF OR destroyed OR sentenced) when:1d`},
	// Africa: regional cannabis names as well as general terms.
	{"KE:en", `(bhang OR marijuana OR cannabis OR kush) (arrested OR seized OR NACADA OR uprooted OR destroyed OR court OR jailed OR sentenced) when:1d`},
	{"NG:en", `("Indian hemp" OR cannabis OR marijuana OR skunk) (NDLEA OR arrested OR seized OR destroyed OR farm OR convicted OR sentenced) when:1d`},
	{"ZA:en", `(dagga OR mbanje OR matekoane OR cannabis OR marijuana) (seized OR arrested OR raid OR uprooted OR destroyed OR convicted OR sentenced) when:1d`},
	{"MA:fr", `(cannabis OR chira OR haschich OR haschisch OR "résine de cannabis" OR kif) (saisie OR saisi OR arrestation OR interpellé OR démantèlement OR condamné OR détruit) when:1d`},
	{"FR:fr", `(cannabis OR "résine de cannabis" OR haschich OR haschisch OR "herbe de cannabis") (saisie OR saisi OR interpellé OR démantèlement OR condamné OR perquisition OR prison OR détruit) when:1d`},
	// Arabic editions: Levant, Egypt/North Africa and the Gulf.
	{"LB:ar", `(حشيش OR الحشيش OR حشيشة OR "القنب الهندي" OR ماريجوانا) (ضبط OR حجز OR القبض OR توقيف OR إتلاف OR مصادرة OR سجن OR إعدام) when:1d`},
	{"EG:ar", `(حشيش OR الحشيش OR "القنب الهندي" OR بانجو) (ضبط OR حجز OR القبض OR توقيف OR إتلاف OR مصادرة OR سجن OR إعدام) when:1d`},
	{"SA:ar", `(حشيش OR الحشيش OR "القنب الهندي" OR ماريجوانا) (ضبط OR حجز OR القبض OR توقيف OR إتلاف OR مصادرة OR سجن OR إعدام) when:1d`},
	{"IL:he", `(קנאביס OR מריחואנה OR חשיש) (נתפס OR נעצר OR מעצר OR פשיטה OR הורשע OR נידון OR שתילים) when:1d`},
	// Southeast and East Asia.
	{"ID:id", `(ganja OR "ladang ganja") (sita OR disita OR ditangkap OR musnahkan OR dimusnahkan OR vonis OR BNN OR penjara) when:1d`},
	{"PH:en", `(marijuana OR cannabis OR kush) (PDEA OR arrested OR seized OR uprooted OR eradication OR "buy-bust" OR convicted OR sentenced) when:1d`},
	{"TH:th", `(กัญชา) (จับกุม OR ยึด OR ตรวจยึด OR ของกลาง OR บุกค้น OR จำคุก OR พิพากษา OR ทำลาย) when:1d`},
	{"SG:en", `(cannabis OR marijuana OR "cannabis resin") (CNB OR arrested OR seized OR executed OR hanged OR sentenced OR caning) when:1d`},
	{"MY:ms", `(ganja OR kanabis) (rampas OR dirampas OR ditahan OR tumpas OR seludup OR musnah OR penjara OR hukuman OR gantung) when:1d`},
	{"VN:vi", `("cần sa") ("bắt giữ" OR "thu giữ" OR "triệt phá" OR "tử hình" OR "xét xử" OR "tuyên án" OR "tiêu hủy") when:1d`},
	{"CN:zh-Hans", `(大麻 OR 大麻花 OR 大麻油 OR 大麻树脂) (查获 OR 抓获 OR 缴获 OR 判处 OR 缉毒 OR 查处 OR 铲除 OR 拘留 OR 逮捕 OR 判刑 OR 死刑) when:1d`},
	{"HK:zh-Hant", `(大麻 OR 大麻花 OR 大麻油 OR 大麻樹脂) (查獲 OR 拘捕 OR 檢獲 OR 判囚 OR 緝毒 OR 搗破 OR 種植 OR 判刑) when:1d`},
	{"TW:zh-Hant", `(大麻 OR 大麻花 OR 大麻油 OR 大麻樹脂) (查獲 OR 逮捕 OR 搜索 OR 起訴 OR 判刑 OR 查扣 OR 栽種) when:1d`},
	{"JP:ja", `(大麻 OR マリファナ) (逮捕 OR 押収 OR 摘発 OR 起訴 OR 判決 OR 栽培) when:1d`},
	{"KR:ko", `(대마 OR 대마초 OR 마리화나 OR 해시시) (검거 OR 압수 OR 구속 OR 기소 OR 선고 OR 재배) when:1d`},
	// No assumed Persian/Pashto/Khmer/Burmese/North Korean local edition.
	{"US:en", `(Iran OR Afghanistan OR "North Korea" OR Myanmar OR Cambodia) (cannabis OR marijuana OR hashish) (seized OR arrested OR sentenced OR executed OR destroyed) when:1d`},
	{"AU:en", `(Fiji OR Samoa OR Tonga OR Kiribati OR Vanuatu OR "Papua New Guinea" OR "Solomon Islands") (marijuana OR cannabis) (arrested OR seized OR uprooted OR court OR sentenced) when:1d`},
	// Europe and the Americas.
	{"CZ:cs", `(konopí OR marihuana OR marihuanu OR pěstírna OR pěstírnu OR hašiš) (policie OR policisté OR zadrželi OR zajistili OR obvinili OR odsoudil OR soud) when:1d`},
	{"DE:de", `(Cannabis OR Marihuana OR Haschisch OR Hanf OR Cannabisplantage OR Cannabispflanzen) (beschlagnahmt OR sichergestellt OR festgenommen OR verurteilt OR Razzia OR Plantage) when:1d`},
	{"IT:it", `(cannabis OR marijuana OR hashish OR canapa) (sequestro OR sequestrati OR arrestato OR arrestati OR condannato OR piantagione) when:1d`},
	{"NL:nl", `(cannabis OR hennep OR wiet OR hasj OR hennepkwekerij OR wietplantage) ("in beslag" OR opgepakt OR aangehouden OR ontmanteld OR veroordeeld OR kwekerij) when:1d`},
	{"PL:pl", `(marihuana OR marihuany OR konopie OR konopi OR haszysz) (zabezpieczyli OR zatrzymali OR zatrzymany OR plantacja OR skazany) when:1d`},
	{"RO:ro", `(canabis OR cannabis OR marijuana OR hașiș OR hashish) (captură OR confiscat OR reținut OR arestat OR condamnat OR cultură) when:1d`},
	{"GR:el", `(κάνναβη OR κάνναβης OR χασίς) (συλλήψεις OR συνελήφθη OR κατασχέθηκαν OR φυτεία OR καταδικάστηκε) when:1d`},
	{"UA:uk", `(канабіс OR канабісу OR марихуана OR марихуани OR коноплі OR конопель OR гашиш) (вилучили OR затримали OR засудили OR плантація) when:1d`},
	{"SE:sv", `(cannabis OR marijuana OR hasch OR haschisch) (beslag OR gripen OR gripna OR dömd OR odling) when:1d`},
	{"ES:es", `(marihuana OR hachís OR cannabis) (incautan OR incautación OR detenidos OR desmantelan OR "Guardia Civil" OR condenado OR intervenido OR prisión) when:1d`},
	{"RU:ru", `(марихуана OR марихуаны OR конопля OR конопли OR гашиш OR гашиша) (изъяли OR задержали OR задержан OR осужден OR осуждён OR приговор OR плантация OR уничтожили) when:1d`},
	{"TR:tr", `(esrar OR kenevir OR skunk OR "Hint keneviri") ("ele geçirildi" OR gözaltı OR operasyon OR tutuklandı OR imha OR hapis) when:1d`},
	{"BR:pt-419", `(maconha OR haxixe OR skunk) (apreensão OR apreendida OR apreendidos OR preso OR presos OR erradicação OR "pés de maconha" OR condenado OR destruídos) when:1d`},
	{"MX:es-419", `(marihuana OR mariguana OR cannabis OR hachís) (aseguran OR aseguramiento OR decomiso OR detenidos OR erradicación OR plantío OR sentenciado OR destruyen) when:1d`},
	// Additional supported editions; Danish and Croatian redirected in RSS probes.
	{"PT:pt-150", `(canábis OR cannabis OR canabis OR haxixe OR liamba) (apreensão OR apreendido OR detido OR detidos OR condenado OR cultivo OR plantação) when:1d`},
	{"BG:bg", `(канабис OR марихуана OR коноп OR хашиш) (иззеха OR задържаха OR задържан OR осъден OR плантация OR унищожиха) when:1d`},
	{"SK:sk", `(kanabis OR marihuana OR marihuany OR konope OR hašiš) (polícia OR zadržali OR zaistili OR obvinený OR odsúdený) when:1d`},
	{"HU:hu", `(kannabisz OR marihuána OR kender OR hasis) (lefoglalt OR elfogták OR rendőrség OR ültetvény OR ítélték) when:1d`},
	{"RS:sr", `(канабис OR марихуана OR марихуане OR konoplja OR marihuana OR hašiš) (заплењено OR ухапшен OR полиција OR zaplenili OR uhapsili OR osuđen) when:1d`},
	{"SI:sl", `(konoplja OR konoplje OR marihuana OR hašiš) (zasegli OR prijeli OR policija OR obsojen OR nasad) when:1d`},
	{"NO:no", `(cannabis OR marihuana OR hasj OR hasjolje) (beslag OR pågrepet OR siktet OR dømt OR plantasjen) when:1d`},
	{"FI:fi", `(kannabis OR kannabista OR marihuana OR hasis) (takavarikko OR takavarikoi OR poliisi OR tuomittiin OR kasvattamo) when:1d`},
	{"LT:lt", `(kanapės OR kanapių OR marihuana OR hašišas) (sulaikė OR sulaikytas OR konfiskavo OR policija OR nuteistas) when:1d`},
	{"LV:lv", `(kaņepes OR kaņepju OR marihuāna OR marihuānas OR hašišs) (aizturēja OR aizturēts OR atsavināja OR policija OR notiesāts) when:1d`},
	// Slang and product descriptions that need not contain the word cannabis.
	{"GB:en", `(weed OR pot OR ganja OR sinsemilla OR sensimilla) (police OR customs OR court) (seized OR arrested OR raided OR charged OR sentenced OR jailed) when:1d`},
	{"US:en", `("hash oil" OR "THC oil" OR "THC gummies" OR "THC edibles" OR "THC vapes" OR "THC cartridges" OR "cannabis edibles" OR "cannabis concentrate" OR "CBD flower") (seized OR arrested OR raid OR charged OR convicted OR sentenced) when:1d`},
	{"BR:pt-419", `(cannabis OR canábis OR canabis OR maconha OR haxixe OR prensado OR "flor de cannabis") (apreensão OR apreendida OR apreendidos OR preso OR presos OR condenado) when:1d`},
	// Native editions probed live in November 2026 (each kept its own ceid). Cameroon,
	// Côte d'Ivoire, Morocco and Tunisia redirect to FR/EG; Nepal, Sri Lanka, Croatia,
	// Iran and Kazakhstan to US/RU; Denmark to Norway, so those stay country groups below.
	{"US:en", `("illegal grow" OR "illegal marijuana grow" OR "marijuana grow" OR "cannabis grow" OR "unlicensed dispensary" OR "illegal dispensary") (raid OR seized OR arrested OR charged OR plants OR sentenced) when:1d`},
	{"CA:en", `(cannabis OR marijuana OR "magic mushrooms" OR hashish OR "illegal dispensary") (seized OR arrested OR charged OR raid OR "search warrant" OR sentenced OR convicted) when:1d`},
	{"CA:fr", `(cannabis OR marijuana OR haschich OR "plants de cannabis") (saisie OR saisi OR arrêté OR arrestation OR perquisition OR accusé OR condamné OR démantèlement) when:1d`},
	{"GB:en", `(cannabis OR "cannabis factory" OR "cannabis farm" OR skunk) (police OR raid OR seized OR arrested OR charged OR jailed OR sentenced) when:1d`},
	{"IE:en", `(cannabis OR "cannabis herb" OR "cannabis resin" OR "grow house") (Gardaí OR seized OR seizure OR arrested OR charged OR court OR jailed OR sentenced) when:1d`},
	{"AU:en", `(cannabis OR marijuana OR "hydroponic" OR "cannabis crop") (police OR seized OR arrested OR charged OR raid OR "drug operation" OR court OR jailed OR sentenced) when:1d`},
	{"NZ:en", `(cannabis OR marijuana OR "cannabis plants" OR "drug operation") (police OR seized OR arrested OR charged OR raid OR court OR jailed OR sentenced) when:1d`},
	{"AR:es-419", `(marihuana OR cannabis OR "plantas de marihuana" OR porro) (secuestro OR secuestraron OR detenido OR detenidos OR allanamiento OR incautaron OR condenado) when:1d`},
	{"CO:es-419", `(marihuana OR cannabis OR cripa OR creepy OR "cultivos de marihuana") (incautan OR incautación OR capturado OR capturados OR decomiso OR erradicación OR condenado) when:1d`},
	{"CL:es-419", `(marihuana OR cannabis OR "plantas de cannabis" OR cripy) (incautan OR incautación OR detenidos OR decomiso OR "OS7" OR PDI OR condenado OR formalizado) when:1d`},
	{"PE:es-419", `(marihuana OR cannabis OR "cannabis sativa" OR "hierba") (incautan OR decomiso OR detenidos OR intervenidos OR erradicación OR sentenciado OR condenado) when:1d`},
	{"VE:es-419", `(marihuana OR cannabis OR "cripy") (incautan OR incautaron OR detenidos OR aprehendidos OR decomiso OR "FANB" OR condenado) when:1d`},
	{"CU:es-419", `(marihuana OR cannabis OR "picadura de marihuana") (ocupan OR ocupada OR incautan OR detenidos OR Aduana OR sancionado OR condenado OR "tribunal") when:1d`},
	{"AT:de", `(Cannabis OR Marihuana OR Haschisch OR Cannabisplantage OR Hanfplantage) (sichergestellt OR beschlagnahmt OR festgenommen OR angezeigt OR verurteilt OR Razzia) when:1d`},
	{"CH:de", `(Cannabis OR Marihuana OR Haschisch OR Hanf OR Hanfanlage OR Indooranlage) (sichergestellt OR beschlagnahmt OR festgenommen OR verhaftet OR verurteilt OR Kantonspolizei) when:1d`},
	{"CH:fr", `(cannabis OR haschich OR marijuana OR chanvre) (saisie OR saisi OR interpellé OR arrêté OR condamné OR "police cantonale") when:1d`},
	{"BE:fr", `(cannabis OR haschich OR "plantation de cannabis" OR marijuana) (saisie OR saisi OR interpellé OR arrêté OR démantelé OR perquisition OR condamné) when:1d`},
	{"BE:nl", `(cannabis OR wiet OR hasj OR cannabisplantage OR "cannabisplantage") ("in beslag" OR opgepakt OR aangehouden OR opgerold OR veroordeeld OR huiszoeking) when:1d`},
	{"EE:et", `(kanep OR kanepit OR kanepi OR marihuaana OR hašiš) (politsei OR kinni OR konfiskeeris OR süüdi OR vangi OR kasvandus OR kohus) when:7d`},
	{"MY:en", `(ganja OR cannabis OR marijuana OR "cannabis oil") (seized OR arrested OR remanded OR charged OR sentenced OR "death sentence" OR whipping) when:1d`},
	{"AE:ar", `(حشيش OR الحشيش OR "القنب الهندي" OR ماريجوانا OR "مواد مخدرة") (ضبط OR القبض OR إحباط OR تهريب OR سجن OR إعدام OR مصادرة) when:1d`},
	{"GH:en", `(marijuana OR cannabis OR "Indian hemp" OR wee) (NACOC OR arrested OR seized OR destroyed OR court OR jailed OR sentenced) when:1d`},
	{"UG:en", `(marijuana OR cannabis OR bhang OR enjaga OR njaga) (arrested OR seized OR destroyed OR court OR jailed OR sentenced) when:1d`},
	{"TZ:en", `(bhang OR marijuana OR cannabis OR bangi) (DCEA OR arrested OR seized OR destroyed OR court OR jailed OR sentenced) when:1d`},
	{"ZW:en", `(mbanje OR marijuana OR cannabis OR dagga OR "Indian hemp") (ZRP OR arrested OR seized OR court OR jailed OR sentenced) when:1d`},
	{"ET:en", `(cannabis OR marijuana OR hashish OR "Indian hemp") (Ethiopia OR Ethiopian OR "Addis Ababa") (seized OR arrested OR police OR court OR sentenced) when:7d`},
	{"NA:en", `(cannabis OR dagga OR marijuana OR "cannabis plants") (Nampol OR police OR arrested OR seized OR court OR sentenced) when:1d`},
	{"BW:en", `(dagga OR cannabis OR marijuana) (police OR BPS OR arrested OR seized OR court OR jailed OR sentenced) when:1d`},
	{"SN:fr", `(chanvre OR "chanvre indien" OR yamba OR cannabis OR haschich OR "kush") (saisie OR saisi OR arrêté OR interpellé OR démantelé OR OCRTIS OR condamné OR Douane) when:1d`},
}

// Country names are query terms, not assumed Google editions. A separate query
// per place stops larger markets exhausting a regional feed's result window.
// Regional aliases are curated from CFI's sourced language atlas/glossary;
// recognition vocabulary also includes ambiguous/synthetic terms, so never copy it wholesale.
// Seven days allows for sparse publication and delayed indexing. Research still
// establishes the event country, substance and scope from opened sources.
type googleNewsCountryGroup struct {
	edition, cannabis, actions string
	places                     []string
}

var googleNewsCountryGroups = []googleNewsCountryGroup{
	{"KE:en", `cannabis OR marijuana OR bhang OR bangi OR "Indian hemp"`,
		`seized OR recovered OR arrested OR raid OR uprooted OR destroyed OR convicted OR sentenced OR jailed`,
		[]string{"Ghana", "Uganda", "Tanzania", "Rwanda", "Burundi", "Ethiopia", "Somalia", "Somaliland", `"South Sudan"`, "Sudan", "Liberia", `"Sierra Leone"`, "Gambia", "Seychelles", "Mauritius"}},
	{"ZA:en", `cannabis OR marijuana OR dagga OR chamba OR mbanje OR matekoane OR suruma`,
		`seized OR recovered OR arrested OR raid OR uprooted OR destroyed OR convicted OR sentenced OR jailed`,
		[]string{"Malawi", "Zambia", "Zimbabwe", "Botswana", "Namibia", "Lesotho", `Eswatini OR Swaziland`}},
	{"FR:fr", `cannabis OR haschich OR haschisch OR hachich OR "chanvre indien" OR yamba OR diamba OR rongony OR zamal`,
		`saisie OR saisi OR saisis OR interpellé OR interpellés OR arrêté OR condamné OR détruit OR démantelé OR prison`,
		[]string{"Sénégal", "Guinée", `"Guinée-Bissau"`, `"Côte d'Ivoire" OR ivoirien`, "Mali", "Niger", "Tchad", "Bénin", "Togo", "Cameroun", "Gabon", `"République du Congo" OR "Congo-Brazzaville"`, `"République démocratique du Congo" OR RDC OR "Congo-Kinshasa"`, `"Burkina Faso"`, `Centrafrique OR "République centrafricaine"`, `"Guinée équatoriale"`, "Djibouti", "Comores", "Madagascar", `"La Réunion"`, "Mauritanie", "Algérie", "Tunisie"}},
	{"PT:pt-150", `cannabis OR canábis OR canabis OR maconha OR haxixe OR liamba OR diamba OR suruma OR soruma`,
		`apreensão OR apreendida OR apreendido OR apreendidos OR detido OR detidos OR preso OR condenado OR destruída OR plantação`,
		[]string{"Angola", `Moçambique OR Mozambique`, `"Cabo Verde"`, `"Guiné-Bissau"`, `"São Tomé"`, `"Timor-Leste"`}},
	{"SA:ar", `حشيش OR الحشيش OR "القنب الهندي" OR ماريجوانا OR بانجو`,
		`ضبط OR حجز OR القبض OR توقيف OR إتلاف OR مصادرة OR سجن OR إعدام`,
		[]string{"الأردن", `فلسطين OR غزة`, "سوريا", "العراق", "الكويت", "البحرين", "قطر", "الإمارات", `عمان OR عُمان`, "اليمن", "ليبيا", "الجزائر", "تونس", "المغرب", "موريتانيا", "السودان"}},
	{"IN:en", `cannabis OR marijuana OR ganja OR charas OR charras OR hashish OR "Kerala ganja"`,
		`seized OR recovered OR arrested OR raid OR uprooted OR destroyed OR convicted OR sentenced OR jailed OR executed`,
		[]string{"Nepal", `"Sri Lanka"`, "Bhutan", "Maldives", "Afghanistan", "Iran", "Iraq", "Jordan", "Israel"}},
	// Native editions for these languages are not assumed; explicit English fallbacks.
	{"SG:en", `cannabis OR marijuana OR marihuana OR ganja OR hashish`,
		`seized OR recovered OR arrested OR raid OR uprooted OR destroyed OR convicted OR sentenced OR jailed OR executed OR hanged`,
		[]string{"Cambodia", "Laos", `Myanmar OR Burma`, "Brunei", "Mongolia", `"North Korea"`, `Macau OR Macao`, `"Timor-Leste"`}},
	{"RU:ru", `каннабис OR марихуана OR марихуаны OR конопля OR конопли OR гашиш OR гашиша OR анаша OR анаши`,
		`изъяли OR изъято OR задержали OR задержан OR осужден OR осуждён OR приговор OR уничтожили OR плантация`,
		[]string{`Казахстан OR Казахстане`, `Кыргызстан OR Киргизия OR Кыргызстане`, `Узбекистан OR Узбекистане`, `Таджикистан OR Таджикистане`, `Туркменистан OR Туркменистане`, `Армения OR Армении`, `Азербайджан OR Азербайджане`, `Грузия OR Грузии`, `Беларусь OR Беларуси`, `Молдова OR Молдове`, `Абхазия OR Абхазии`, `Приднестровье OR Приднестровья`}},
	{"GB:en", `cannabis OR marijuana OR marihuana OR hashish OR hemp`,
		`seized OR arrested OR raid OR uprooted OR destroyed OR charged OR prosecuted OR convicted OR sentenced OR jailed OR killed`,
		[]string{"Albania", "Kosovo", `"North Macedonia"`, "Montenegro", `"Bosnia and Herzegovina" OR Bosnia`, "Croatia", "Estonia", "Denmark", "Iceland", "Malta", "Cyprus", `"Northern Cyprus"`, "Luxembourg", "Liechtenstein", "Andorra", "Monaco", `"San Marino"`, `"Faroe Islands"`, "Greenland", `"Isle of Man"`, "Jersey", "Guernsey", "Gibraltar"}},
	{"DE:de", `Cannabis OR Marihuana OR Haschisch OR Hanf OR Cannabispflanzen`,
		`beschlagnahmt OR sichergestellt OR festgenommen OR verurteilt OR Razzia OR Plantage`,
		[]string{`Österreich OR österreichisch`, `Schweiz OR Schweizer`, "Liechtenstein", `"Südtirol" OR Bozen`}},
	{"FR:fr", `cannabis OR haschich OR haschisch OR "résine de cannabis" OR zamal OR pakalolo`,
		`saisie OR saisi OR saisis OR interpellé OR arrêté OR condamné OR perquisition OR prison OR détruit`,
		[]string{`Belgique OR belge`, "Luxembourg", "Haïti", "Guadeloupe", "Martinique", "Guyane", `"Polynésie française" OR Tahiti`, `"Nouvelle-Calédonie"`}},
	{"MX:es-419", `marihuana OR mariguana OR cannabis OR hachís OR mota OR "cannabis sativa"`,
		`incautan OR incautación OR incautaron OR decomiso OR decomisaron OR detenidos OR capturados OR erradicación OR plantación OR plantío OR condenado OR sentenciado`,
		[]string{"Guatemala", "Honduras", `"El Salvador"`, "Nicaragua", `"Costa Rica"`, "Panamá", "Cuba", `"República Dominicana"`, `"Puerto Rico"`, "Colombia", "Venezuela", "Ecuador", "Perú", "Bolivia", "Paraguay", "Uruguay", "Argentina", "Chile", `"Guinea Ecuatorial"`}},
	{"US:en", `cannabis OR marijuana OR marihuana OR ganja OR hashish OR hemp`,
		`seized OR recovered OR arrested OR raid OR uprooted OR destroyed OR charged OR convicted OR sentenced OR jailed`,
		[]string{"Jamaica", "Belize", `"Trinidad and Tobago"`, "Barbados", "Bahamas", "Bermuda", "Grenada", "Dominica", `"Saint Lucia" OR "St Lucia"`, `"Saint Vincent" OR "St Vincent"`, `"Saint Kitts" OR "St Kitts"`, "Antigua", "Guyana", "Suriname", `"Cayman Islands"`, `"British Virgin Islands"`, `"US Virgin Islands" OR "U.S. Virgin Islands"`, `"Turks and Caicos"`, "Aruba", `Curaçao OR Curacao`}},
	{"AU:en", `cannabis OR marijuana OR marihuana OR ganja OR "Indian hemp"`,
		`seized OR arrested OR raid OR uprooted OR destroyed OR charged OR convicted OR sentenced OR jailed`,
		[]string{"Fiji", "Samoa", "Tonga", "Kiribati", "Vanuatu", `"Papua New Guinea" OR PNG`, `"Solomon Islands"`, "Palau", "Nauru", "Tuvalu", "Micronesia", `"Marshall Islands"`, `"Cook Islands"`, "Niue", `"Northern Mariana Islands"`, "Guam"}},
}

var DefaultGoogleNewsQueries = defaultGoogleNewsQueries()

func defaultGoogleNewsQueries() []GoogleNewsQuery {
	groups := [][]GoogleNewsQuery{googleNewsLanguageQueries}
	for _, group := range googleNewsCountryGroups {
		queries := make([]GoogleNewsQuery, 0, len(group.places))
		for _, place := range group.places {
			queries = append(queries, GoogleNewsQuery{group.edition,
				fmt.Sprintf("(%s) (%s) (%s) when:7d", place, group.cannabis, group.actions)})
		}
		groups = append(groups, queries)
	}
	// Interleave regions and language-wide queries, including on the first scan.
	var queries []GoogleNewsQuery
	for rank, more := 0, true; more; rank++ {
		more = false
		for _, group := range groups {
			if rank < len(group) {
				queries = append(queries, group[rank])
				more = true
			}
		}
	}
	return queries
}
