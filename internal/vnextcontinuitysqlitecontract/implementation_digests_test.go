package vnextcontinuitysqlitecontract

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestContinuitySQLiteContractPinsExactPersistenceImplementation(t *testing.T) {
	t.Parallel()

	wantSources := expectedPersistenceSourceDigests()
	wantFunctions := expectedPersistenceFunctionDigests()
	if !reflect.DeepEqual(sortedMapKeys(wantSources), sortedMapKeys(wantFunctions)) {
		t.Fatalf("source/function digest inventories differ: sources=%v functions=%v", sortedMapKeys(wantSources), sortedMapKeys(wantFunctions))
	}
	for fileName, wantSource := range wantSources {
		if got := digestSourceFile(t, filepath.Join(sqliteSourceRoot, fileName)); got != wantSource {
			t.Errorf("%s exact source digest = %q, want %q", fileName, got, wantSource)
		}
		gotFunctions := digestQualifiedFunctions(t, filepath.Join(sqliteSourceRoot, fileName))
		if !reflect.DeepEqual(gotFunctions, wantFunctions[fileName]) {
			t.Errorf("%s exact function digests = %#v, want %#v", fileName, gotFunctions, wantFunctions[fileName])
		}
	}
}

func expectedPersistenceSourceDigests() map[string]string {
	return map[string]string{
		"admission.go":                     "5d14f6a57ac5d309fccebb60c4e8b2d717118eab231fd9399bbff89ae62d68cf",
		"append_kernel.go":                 "fbc21e0c0e89392b6138e44261f6a5765af9b79420790fae66444351fa486dd9",
		"append_methods.go":                "b6ffd99675243104c812d16e96b1bc0f2b7150c4b87da2881ba96751760e14ea",
		"codec_v1.go":                      "08c0a0ac686e3d5c8fc8523949701eded5bb2c858c2d17d927225d1e6acd9c86",
		"driver.go":                        "bd8fc591dd2d5c9c2c8f5f9ea6320b913a08ad8b9e1f9243c4e1a93538970a44",
		"filesystem_attributes_windows.go": "1e3b5f12e4debbc5432f2153818862cdc39ec617409e526b750e6db093dcb0a0",
		"filesystem_unix.go":               "4a06e1818660145b50a3a28a7c0b4e994df0eb5fc46bed56d1f424259000b0e2",
		"filesystem_windows.go":            "0770148405c2b7f215a92e3706443bd6bf6438790c926f6c5a18461a09628818",
		"schema.go":                        "5eb35bcf96fed17242202e75b24af1a82555b42c3079a87d4d491d67051d57ab",
		"store.go":                         "07fb4c03e28e989b2f8d3155b4899e1d81b3a69e1de4aef48d29fe99f6cbd8a6",
		"wire_v1.go":                       "1b632f0aa5a96077e00ee660ef566a0da204cf004870dda2576b0bf141d32bbd",
		"wire_validation_v1.go":            "783dff45234fa3717969d364c8e9e7d2b78aa420da262b6be230c238b57ba2f9",
	}
}

func expectedPersistenceFunctionDigests() map[string]map[string]string {
	return map[string]map[string]string{
		"admission.go": {
			"admitDecisionSuccessorV1":       "36d3734c8367b2d6ddb5406e8954963f70efc4db2b7d27ade326ea550177642b",
			"admitExternalReferenceEdgeV1":   "be927383ebd4532c56dfe615c92b748130e8a128e00b16cb2026791048cb7afa",
			"admitFactV1":                    "ef33da2795eefd328ff87cda06a6404eec0df61bb211a947d3d45e594bffbbd0",
			"admitNewSubjectWithFocusV1":     "d82046d62f0b98ed2c49b2a3739f58d8d611bb699d4197b51d732f5b048875b9",
			"admitProjectRegistrationV1":     "2da8e246adc7556c008e086fa5c348c98cfdb6e0eb84a32ec7b6ee16d7c5670c",
			"admitScratchpadFactV1":          "b443969c194a6bf6b57a9fec35688bf039384b46206940bbc4d1d2dcd16cd513",
			"classifyPredecessorMismatchV1":  "ef2f5b9554b1ffd8df8ed92adb97ecafa142e72ba2c203cf78530edd85589806",
			"containsFactKindV1":             "203832a102d09187ca526f65e06ff546f48e68d248ed5f0c65f56c5388589a39",
			"foldExternalEdgeV1":             "b65111354cbe7706538ec807b2fe066aa73c057cecc3091b815d43d7088969a4",
			"foldScratchpadStateV1":          "45a799112c05a4ec983de33fd3d499d5d37b73ee6cb5180843fa627f19b58acb",
			"loadSubjectFactsV1":             "9492d34709ca7b2a0f7e5869c6009ff61f52e5caff37f271dd89c9cb4d9d70f7",
			"requireActivePredecessorV1":     "b79777d4cdd0595c198173a2b66e82b22cb2407df6615e001e6a2ea74b585015",
			"requireCurrentPredecessorV1":    "0012d6d119b7af36138da2047d28c8d525c8b82c166d59f0aaf17414ecb69551",
			"requireExactEdgePredecessorV1":  "bf8be4523de0fac5897335f0803fb9b63dcb5b05d1adf14b532f6dbeec62a773",
			"requireNewSubjectV1":            "a501e12315cec6cdbf762e5a208fa6de577dc4c2b35ffe73c8745ac469db40c8",
			"requireProjectV1":               "e2569ac7bb5b22ca816faa423d28eb020ff76e4c02cb49f0551303ff41f7f9c6",
			"requireReferenceV1":             "6e7739edf5a26c18c5bf86fc40b187c8b204a95d9b47d30e6a168b08c580e5b3",
			"requireScratchpadParticipantV1": "9213282b4962c87bac7c54d2a64b3cad5460e5270cf0059b63328cd2f6e9ad67",
			"rootFactKindV1":                 "76bc045fa5deb4c824e224bbef0b0a6b8459a54a463e8d36ab738937b35b5061",
			"validateSubjectHistoryV1":       "9b2b39d5122479a4495b33fd2a0af0bcc8bf8afc2e36a3a10c5f549532ad13ec",
		},
		"append_kernel.go": {
			"Store.appendFactV1":            "ac0924bd06e2e2af440f2db232aad99c89bb50aef94cb9113d17c5580a3ab41e",
			"allocateEnvelopeV1":            "9bf3f41c2e97be809f02d95b9f329e3689659f4d01c22f9cd16417ab02672dd1",
			"corruptFactProblemV1":          "57f39bd5a215cf84386d7d7b6b92bcd1276eed50a27ee01097e3853a69433487",
			"finishStoredFactScanV1":        "ed8b1b40f9ce0076291e3e1238626e33c975f7d8c34469c12462f55a88e44dbb",
			"insertFactV1":                  "df0b01d0dd779e085c923931e739a1a39efd503aaf8588736050a98c2fdece6c",
			"readFactByIDV1":                "044c20c4d8b7976ab51a639943df06ccebfbcefdd533d2c006861f99daeecb8c",
			"refieldProblemV1":              "067f4bb3f09ceb6205f5d20b1687909bfcd7aa6205330c97e78b92ca9ad78c49",
			"scanStoredFactRowV1":           "c059b5496ee32bc28a0a7a3ac059731223c9003dc4a0cc99160061f820b61f2e",
			"scanStoredFactRowsV1":          "118ee73a6365fe8cab39fb0cdc0b81873ff234a4b5b4df0b97eec388b6d3b08f",
			"storeClosedProblemV1":          "4df446ed694e303c05ba90f4a3249b22a6994c55a3f2ef49a296d095cd8b926c",
			"storeUnavailableProblemV1":     "82b1059fc52e07ce69bf4f31a8dc83726a2c2f2006af4c25c010afb15647b30c",
			"storedFactV1.matchesIntent":    "f676457ac8c730700e9c4f95285e9b2182e7cd0394b7fd43f76c40a75d083ccd",
			"storedFactV1.receipt":          "288c4885ceb3ef94a5b61a05b1c141346dcafea39617ac4e1e733e14e7e28f03",
			"transactionOperationProblemV1": "2558e0361e1298fd151eae00be0936b06002186a88e5b38a61ae8de8d8972b65",
			"validateAppendIntentV1":        "5bcc22b040e334df1a1e66e97dbf6f8340688c03254fd62029a28b8a4e3eda5e",
			"validateStoredFactV1":          "75150a070264e9d7a7e2df85103acf2423d503e6f130733f47546072a25262fe",
		},
		"append_methods.go": {
			"Store.ArchiveIdea":                    "0026c114b3dd296ad61608335a03ae6bbe25fb60bdfa07f861d9f4a4833a4db8",
			"Store.AttachExternalReference":        "a1cf0a62dd6696c6b37cc00f83bd3a61ab48e334d117788bce614612174ea59f",
			"Store.CaptureSpark":                   "5aa585a8c5c49f97c0e1078ec1b978a9f40dd04a2b8746c10e2e0746837f0ec2",
			"Store.CloseScratchpad":                "0d7e7d46a8d1bbca69258672e9716c59d927d157778e4633280be0e01971a275",
			"Store.CorrectFinding":                 "87f162fa4368ca07ab845d4ad1f095137d01e1cc93220996e2691a723bb624de",
			"Store.CorrectJournalEntry":            "2411e27985ee8989f7b6bcbd37bf5a04343de3b93077bca4334825947f28ce13",
			"Store.CreateIdea":                     "68859ddc7350a447c33afd13af2c007cf406ba2a96e85caf2608d3045ffa9a04",
			"Store.DetachExternalReference":        "0a2c28c16ed1ee52e88b02df6e90c2de7bf686fa10c985b08ae3001a95c4f545",
			"Store.DismissSpark":                   "c4b45d6bd22c3be971d3fa9776c8996d3dd4e7db10f55d6f3c2f28ad442c8932",
			"Store.IntroduceScratchpadParticipant": "8505d073cc1aa48658d6d00fa2618bfc2e85ba679998785f9197b2fcf1e4cf8a",
			"Store.OpenDecision":                   "d91b1eaf286b3a75876c088fc7d958eb30be5da3253130d506a959aa3c1337f6",
			"Store.OpenScratchpad":                 "c6d1e91215c7e23c02ceba239cfea9065cee551d1dca546eec5ebe843bc88a2d",
			"Store.PromoteIdeaToExternalReference": "2a534eb480aa56d1c5e6765d1263a1316f2baab0c194579ad1fe4569dc0a1016",
			"Store.PromoteSparkToIdea":             "3aaed4eaf6f2423ca8bdcff8143114ec8c5f672a96fe17c1a547de1c8e4429d4",
			"Store.RecordCheckpoint":               "0524179efe4b741f83d2e432f53f2e90187b75e497e1683a22bd69b496b91057",
			"Store.RecordFinding":                  "6438dac8ed742df51164db780f395adb77240be1fc09cda5d4a12aa4dca07a53",
			"Store.RecordHandoff":                  "54a7f8caf0b40774b658456af76534b7ef7312b5a12e1f60e255adef0468873f",
			"Store.RecordJournalEntry":             "df830bb70d8e3e1549faa76e026d621612b06c19246286790d6772066e16c15d",
			"Store.RecordScratchpadClaim":          "02585c082faad67284e6897b38b0dc64344e7940833fd88816a0f827062e9013",
			"Store.RecordScratchpadMessage":        "43ac99ee84f8865cf7444a14e2e0d783f06446245fc4b16fc285395dda14339e",
			"Store.RecordVerificationEvidence":     "5dd22fc978fd872748e65cfd47e398c87541c2a398d4ccf3fd6fb18787639304",
			"Store.RecordWrap":                     "27948f4259a0779d15e724b7129a0ba831f45bb191142b44dcc52f9710518204",
			"Store.RegisterExternalReference":      "c333ade4d515e75222a053c01da264d34e6532700099dc4b8ae89d82f01cf645",
			"Store.RegisterProject":                "b92ce58b23b91e0579cb719753c039926476616138409b326221ea9e63b70212",
			"Store.ReleaseScratchpadClaim":         "fdce5925f9ab3e87432bdfb8b3ce6a8bba78e31d5b988805d4c218f5e336f982",
			"Store.ResolveDecision":                "93f5eaa6025c7f3d294e196e07a4ac157bf7681e940b421cccfa0f807b44cf06",
			"Store.ResolveIdea":                    "8017af4270cec04ee8dda11ae6cca8546afd1d882ed4ff3a185a5dd30a1bc9d5",
			"Store.RetractFinding":                 "11775f27a18b17406589e10fcefad80aeb5e3bc66f75d1b731eb9fdeca402259",
			"Store.ReviseIdea":                     "4f3620a61bcdcb4890855315a97eda42dedab24107759ede5b42f2fa30ed7824",
			"Store.ReviseProjectLabel":             "b25fb8540a52641dbb351f226aa58524c827599fbe73b5d40a4e46746782a262",
			"Store.StartExploration":               "788cfc8bb0c410de13fb55b5eb32db7722b1f9cc2c159f2368a50fafde4533af",
			"Store.SupersedeDecision":              "b9d3a36dc4de556c8fc0eea657e557ecb78fdc1c3a0f76c4f87766d7779fed86",
		},
		"codec_v1.go": {
			"canonicalizeStoredContentV1":           "6cd929ed469ac8e435a75f2bed401f09ae9d730092753e6d27db6c8c778b59df",
			"canonicalizeWireV1":                    "12c6bd6b57d0582c613a8df4f67add0d320eaec84315f14f700072d2b69b75ba",
			"corruptContentV1":                      "e7b2cc699ff77896b3827433cb6290114cb4cd9c97b28c04b659c93354d89c9e",
			"decodeStoredWireV1":                    "f7dd732809cda8e43f87c8e70a6b40250d19e40e3ef43e40cabe728617b499c1",
			"decodeWireV1":                          "34f76f1f9c5c1cd3812f4449efd4d35a65b3ad07ada89fa8d20e1f100c5dcd49",
			"encodeCheckpointRecordedV1":            "236e5373d0f472d292356c603503134eacc0acb729d3201c576da015518dbf04",
			"encodeDecisionOpenedV1":                "59ec933b7e184253b6b0eafb74c88ab82e0a71ad9409865c8f44ab8d447e679b",
			"encodeDecisionResolutionV1":            "dcc8b3692d3d744f16f273f0c668059fa1ae127d1f4e1babad100c50c14db7d2",
			"encodeDecisionSupersessionV1":          "2e985b77a73edf97a1f5b0b971e958e1ecb2d5205ce235d04c1ea231ab22d233",
			"encodeExplorationStartedV1":            "f1516e543b3cec65e8ad9d8464bbd1effe83ce8521b62cc9c0c2a6a2b6a52ed3",
			"encodeExternalReferenceAttachmentV1":   "43f811467c3c859f997e7d490e55fbad6ca098814c78217b4a2366cdd75fcde3",
			"encodeExternalReferenceDetachmentV1":   "6a021fc4fce5e6dfc1b55cfcb172c3743ff17aca15f01af59b820645c3a034fb",
			"encodeExternalReferenceRegistrationV1": "e616f6fd2a36ae2ad70e705b2183775720fda719a31a30431558a3462fdf13fa",
			"encodeFindingCorrectionV1":             "82e64a74c733ef785af5eb3c69d065262790fd26a72ef863aa6bd213e9dbd6b8",
			"encodeFindingRecordedV1":               "a147e70821712b7e896adfe6ef7f6d29476ce8dc89faf0b124a51e21c4c9abd9",
			"encodeFindingRetractionV1":             "5ee7c0dd7a732a328206ea9ef2526ee2fe71a517b672ad6b5fd2c4b854bb0ee7",
			"encodeHandoffRecordedV1":               "e7ff5095e921da7a7704a841c13c2fa31f03bf89aa8c50b6fe3b21a34111a51e",
			"encodeIdeaArchiveV1":                   "3524b2892aedc15de56dcd02fcc41490a5aa6fa363a33253ddcf4bbb919dd22e",
			"encodeIdeaCreatedV1":                   "33b74d1b8545ecb69c3dfcea7119a39bdbead1972251976dabfdef123b88ff78",
			"encodeIdeaPromotionV1":                 "eb325b0270ca49f5b0e302d87f8b050afd0787bdbeb56d56ef5ccd932db15b38",
			"encodeIdeaResolutionV1":                "be8518007bbc17a50ba9e1ebe1196bf1ea0a5b0c8fe7e50dd9bb72c7f1b097d3",
			"encodeIdeaRevisionV1":                  "4ab2a167506f435943cbe20096cf640cc98bcea1d9ee4a97afaf879a2f5dde59",
			"encodeJournalCorrectionV1":             "b941531cc168053d410afe7bf198a546776a6fad8c263ad27dcb5b63fe8a36e5",
			"encodeJournalRecordedV1":               "c691fede97b0de3c8cc9506080312b2a2903265887dc6f813a22bdd65176f287",
			"encodeProjectLabelRevisionV1":          "0c1fc3a1e4d546863f2ee689eda2a232f6516cfd647a8a783dba75355e34162b",
			"encodeProjectRegistrationV1":           "bc10780dff3d7ad62a17e7491becad82ccef599558ca29aea01e37d8878abfd1",
			"encodeScratchpadClaimReleaseV1":        "c3b1cb55d8e80a06141d60f818bbddec2ac117a052bddc27a986053f9a65f1e9",
			"encodeScratchpadClaimV1":               "b534b918c47aa7233b62030334193e739919f7a5eda61be61faa745f51d26382",
			"encodeScratchpadCloseV1":               "7f5369d5aaa2eb0fe0ec72a51ddd18dcea17c586cfaf643552f2bfe8f2b63acf",
			"encodeScratchpadMessageV1":             "f3f3ef1d325e01ccaa483ef3063040aa90bfb74fa4cf112a290263c420207d4c",
			"encodeScratchpadOpenedV1":              "7e85733d45000f71cf4538e09e532ad3ee1417ed1a3e14d0cad37841f72bc65d",
			"encodeScratchpadParticipantV1":         "6d37879d143b5eff607e28841fee1b9d652396077bf164ee40ae6e816842dd1a",
			"encodeSparkCapturedV1":                 "2240e13141db3b6ade5b5bcece00786e24ea34c4182f017dc62755756fd2564a",
			"encodeSparkDismissedV1":                "a6ef703639375027225c870d493032166caf43e224d8f747f461b6f2dad598d8",
			"encodeSparkPromotionV1":                "4034f93b7787c0259d047de57a5d61f7b1ee63bf15e0227b2d89b539085e0403",
			"encodeVerificationEvidenceV1":          "8a4489980bbdd35c61aea01d83d7873bb48e7825a99df78c13fc57d562443d2f",
			"encodeWireV1":                          "509943bb5a171c76bd6db9811b181adab9a00983f7e13d678190e0a5390be19b",
			"encodeWrapRecordedV1":                  "8bfca64960f1b4e5e9775f73e11d90d5c3efec561ed9566a13a5b43cadff22e8",
			"normalizeCheckpointItemsV1":            "b5032e76be82811f80d33d96d377345cafb07a5c21c8993cc6054c3372ef538a",
			"normalizeStringsV1":                    "a778e71e55b404b44196eb92c04b07c3125c283da5892bf6c47e668487379a6d",
			"requireCanonicalV1":                    "a5d6632b6a8ac961fcfce29da216e5b879572624aef289c8b376707c30350a5e",
			"toWireFindingContentV1":                "466f67bcd0aaa037b071f73c7b4d43ee18a657a01d79380d1bc665c381f5f0be",
			"toWireIdeaContentV1":                   "f112471e220bc2a3389b98c54bcb84d32c82d27ca1cbc3dca1943dafcf1870d8",
			"toWireJournalContentV1":                "fedf1952668a5f1e8b0b2032eff4d69f209d98fd24755c11ae5f101d42b730e3",
			"toWireObservationV1":                   "799d9dde98f24d7e17e9ef912172097e525ae69a6df56b1334b705a8d5bbe065",
			"toWireOptionalSubjectRefV1":            "f4ccf25b47609aa4aef11dab31ab705f50e5bb361804f77b6c98f6daf73206e1",
			"toWireSubjectRefV1":                    "c14e92a37beb57cd445cd0c66a1d0c06e73bc3f24611e32a5068abd2d03ab7b1",
		},
		"driver.go": {},
		"filesystem_attributes_windows.go": {
			"windowsReparsePoint": "81f5d3403c7fb9f2fbb37255d5061ae761a50c56fadc53b5aa52c048cc7c0440",
		},
		"filesystem_unix.go": {
			"databaseURLPath":                    "a4fa2923449759d339aa49022c0068bf2cc615e18d558762954c9be63da9365a",
			"securePrivateFilePlatform":          "358bedcdc24452ef3951e35358f97c1656f2490fc78cb734c4e7eb0ecb080133",
			"validateExistingUnixPathComponents": "e6c2bc130ca8d6742805bac6a9d7b518f01eecac2f8060ba3c78a2439d2f13a6",
			"validatePrivateDirectoryPlatform":   "73ddadd3b323b7ff9872b82e41b643d2acf5b1229a86714b7dc406b61d0737a6",
			"validatePrivateFilePlatform":        "bce83a861542a75dc778d2b7f252d19820868a18c7e92acc20052ac41abde4fc",
			"validateStateRootLocationPlatform":  "818cc5d8212da884024e801df3c8273bc5d71ff3a3c856d0c05d7c82377af3ef",
			"validateStateRootPlatform":          "d465119f3cd961c818f763b8540fb4544733f08dc5542953ba8fbeb215dacfc7",
		},
		"filesystem_windows.go": {
			"databaseURLPath":                       "6539227ce439034da79cd08ef3df63b5e89821bd0b8200b75d528fc7dda7e35e",
			"isWindowsUNC":                          "9fc97df4a9d797e9d464c94a93891271ec59c83beafa825db11c28e9b58b8737",
			"pathWithinWindowsRoot":                 "8d2439757c50d9ba3e2f43708cdc0c6ef09469c0113be2f0211231b129076237",
			"securePrivateFilePlatform":             "cfb416d3d80180bb04a15e94dc97241c9eb50016aa988af4c488dee89e0f17eb",
			"validateExistingWindowsPathComponents": "362c51d2e892771ea2de3b2a69a5299777bea8d58f843377af4f117e9aa9dfa9",
			"validateObservedWindowsPath":           "655fc09ffbd4d1633c0c74a7214b0e483ca2156b6d72dcc13a06d09fe3884ea8",
			"validatePrivateDirectoryPlatform":      "b62ff8dbdae49190bf22a1779d687fd53d161792a98d8a9f50e5af732ac89d63",
			"validatePrivateFilePlatform":           "9fd50adfb65959ed2c792e0c3c84dc4b97f16fb677494776f04f02331a6ab1d1",
			"validateStateRootLocationPlatform":     "4b10e1964a7edd4c2b5b0b9bd2169f4f3bf7f893d4bd43716a897a4d4bea644b",
			"validateStateRootPlatform":             "db32888248aa99a7a5ee8f72060f1fc1ae8eb6e07e6c9ac7b19aa6f8eebc6751",
			"validateWindowsPathComponents":         "7613a329580b81601f4f6d08cc74abcab3d132504ed9e7ccebb075a7c38a2a0a",
			"windowsUserDataRoot":                   "97b6595bfcf4d216279ef990542b9b02509a1763f09ac0cfdd1898a69b5bc3d9",
		},
		"schema.go": {
			"checksumSchema":          "e697037955f01095488bb1258a6f4ed1107c40fb10407ad2beb7e473e389efdb",
			"expectedSchemaObjects":   "966a8404232d9612b7c59245b2a709786930097715d60b547ecc962f403ed204",
			"initializeSchemaIfEmpty": "373d4af0aac75988cdb891d1494a6d0560fa0959a9f5635d95a6b0d183dbca09",
			"normalizeSQL":            "ba395a0d4dddb73b4b9669ad5a6cd12d28494ba464560910074abbddd0d96e98",
			"validateSchema":          "0ee48b24d251815fa8ba379118493edf06fd843ea4a3ea6ba41d9febf9fb6079",
		},
		"store.go": {
			"Open":                      "c5e946b2c2be830e01091143f2a8f0e3b19a8428d434b3b31cdf032dfc197971",
			"Store.Close":               "69f3a1056e0ce920d07dbdbf96c0559b9edf375218c904d29ea307d6623006f1",
			"databaseDSN":               "31b0b9b4bd1269c1e5f8c521c17fcac5ed99cfe1609e48e1937744a729788e19",
			"inspectRegularPrivateFile": "177aaa7ec2087d38ca322829f4a54d9c333a51aea70be68823d1d9d34772e64a",
			"openDatabase":              "63865fcf446d578fad398587c9c01c6e7330f6117673125cd4afd351b57c3dfc",
			"prepareDatabaseFile":       "22eb5fd9fe6e793d6f846255ffd83be7e43748b2d41fcac76e4bc6f4322e74fb",
			"preparePrivateDirectory":   "5024c5c56c7bf3b92ba98eaf138d8c78542146bdc992051583772566fb629fa0",
			"retryableOpenError":        "b359c6f459c7490b1f8aed8df8e3f137456003c3a1d5b0a842cc639f53e437ca",
			"schemaIsEmpty":             "d96549881a4a329b5815e73d4b144cbe6b8c3cb49a56b5e98c7736d7532ac4fc",
			"secureSQLiteSidecars":      "403d22fdce31128b08551f01f6cb68102332930ed3c6ebaf0cceaf0d32fb0e89",
			"validOpaqueID":             "c0072295d2bd1701a690020b9a9b3791eba2c22429671460ffac401896d1bdb1",
			"validateEnvironmentID":     "6f4006fba6c237bb97b739529c6a82f739cf3a75758fa97625b36da20389fb67",
			"verifySQLiteFiles":         "32e01c16e83df7de6a0034055df7ff29d169823a2d268fc412b824609ad8891d",
		},
		"wire_v1.go": {},
		"wire_validation_v1.go": {
			"wireCheckpointRecordedV1.validate":            "a95f5ec61397d45bb71912c5324104c00a4b11cc450fd9145c23c192c63d194f",
			"wireDecisionOpenedV1.validate":                "27e96a252ecd982395ba332864f9692bfd9d8a7015ffd604d6ff5a9cd35d8c1b",
			"wireDecisionResolutionV1.validate":            "d246a87c7e446c4a662372a5b23a1bc4b1eec5e7087b64c96844e86b041b111b",
			"wireDecisionSupersessionV1.validate":          "0c5a2be9f292f7e42228a404151a4109e3814972eaee58bd7f5b7d7aea10f782",
			"wireExplorationStartedV1.validate":            "b2b7259cab91d53a9e88529aa79f945476db8581b2300a242b29c713c2f1dfb7",
			"wireExternalReferenceAttachmentV1.validate":   "f99d406311402e7e950f1a884cc4fb13c0729803487d7b876e79e7d5634a7c73",
			"wireExternalReferenceDetachmentV1.validate":   "521054466c5602f8368ce30bb7c9b737e1f115e593ba45defa0d427a8eced3b8",
			"wireExternalReferenceRegistrationV1.validate": "97844e3935ffd64a3f0cf6ac3b6f17821457f67188493c1ad942550c220fcaf6",
			"wireFindingContentV1.domain":                  "9239b36eda10e789a4e3e3f1dc76e81c3d5f983d1398e17189541b7f2a61fbe0",
			"wireFindingCorrectionV1.validate":             "84a05d840cfd7fd089a3ca43716b1bb504456a2651dda59e2e3c29f7d47045fa",
			"wireFindingRecordedV1.validate":               "2c0582c844796f2cd31961c084c423908261454226b4b2574284632c6f40c74f",
			"wireFindingRetractionV1.validate":             "203d9556c15ff881514144a1b019a699d1933dcea3926bedcb67b0664c39b463",
			"wireHandoffRecordedV1.validate":               "076c7d77bce271ed38c2391ccaa806e1e7554c9018254359d116a41ae6aedf61",
			"wireIdeaArchiveV1.validate":                   "396fc69bc6954505390b1a3b85f9a842b7398c1f776ac80f4daa80cf84419d73",
			"wireIdeaContentV1.domain":                     "03950f5df548d83ffe390f0059681f41299f9c4d3210e196b1765e1b06464cc1",
			"wireIdeaCreatedV1.validate":                   "18fca63ab7c4e319e5bca562221f052938a740c08a728e470b7512617899557a",
			"wireIdeaPromotionV1.validate":                 "2b7179524a0849674b5e49182afd0679e94ae741b9a319d2fcab3c08c8d7045e",
			"wireIdeaResolutionV1.validate":                "7c5da7ec5dae7b4270b7078b5785f7c154d71f1e8279042a965b3e2fb2359e18",
			"wireIdeaRevisionV1.validate":                  "e8e0b85366729ffc0f1d78672a73150680500189be48c549579626d7e3570aab",
			"wireJournalContentV1.domain":                  "910a9d2e31f907735d1b3fdd142e5c9d16fd0f4524f29a023f0d2170c1a496c3",
			"wireJournalCorrectionV1.validate":             "3adb3b6befb0f583d49bb49ed0c57289578c3c55804fa41b74e3ab5893d15ac7",
			"wireJournalRecordedV1.validate":               "472d6a740c8185c7542bddbd25fc7f7d28e0db189a7368d70fcf53700d45d2f8",
			"wireObservationV1.domain":                     "2ce9ced1d35695b4ece8e75dcd515ce72c61abd172350554ce238e89c1667e4d",
			"wireProjectLabelRevisionV1.validate":          "eca97d4ee7070cabe6afa96eccd8103bfe024a15b0668cff650f891f348f5877",
			"wireProjectRegistrationV1.validate":           "b525dd000a142832af7bbe62dcdeb1bcb32fd4def0d215964a15cb1c06f218c8",
			"wireScratchpadClaimReleaseV1.validate":        "9e80c4979e1aa7c2090661de4b351e32612ea5a787c25e87a5dbf29e02e4370c",
			"wireScratchpadClaimV1.validate":               "d75f5197a0eb565fe1b011dc8a92d85fcfda5a598deb4ba908e4567e4b36e46a",
			"wireScratchpadCloseV1.validate":               "6acbe24e522e3d4944c831923cdabdaea5d832ba97f2a7a6c8fc7fb4af2af5ef",
			"wireScratchpadMessageV1.validate":             "6863eaba646b3e4239a7d8570519ecf75b98cd020d81f976345a960e4f5fce59",
			"wireScratchpadOpenedV1.validate":              "9d3dbabe844d2fb349e89fe27e6f0960d4e68e34cfaa9db81474ae5aaa41130a",
			"wireScratchpadParticipantV1.validate":         "7766456e83d1e673628346ac02d912737cc1ce3f413e59d742cc2237b711771c",
			"wireSparkCapturedV1.validate":                 "af2ae4b83662c7864c47dcadf2fd55e8cc0fcad1e42843764736555d4bf8cfd8",
			"wireSparkDismissedV1.validate":                "896ff08ddcbe332534feaeeddc129e201b766dc5ae336d90f5bf9d09994c4ef2",
			"wireSparkPromotionV1.validate":                "b83a386970837b2f10a3ba4459d7f65d3ff460e8e52a03bb30a7a0408218116b",
			"wireSubjectRefV1.domain":                      "b1aae2431f356f7f03287c64c623f61462acddb430aa2f504d1b497c27c37d84",
			"wireSubjectRefV1.domainOptional":              "c5b3f4bc108c173612514b33cb825978fb2ff5706f21eff37bd6db4acc6d32b3",
			"wireSubjectRefV1.domainValue":                 "bc63f16ff246a6b7464506b3d197e95f553c65672dcb98a38c1f81569bbc6739",
			"wireVerificationEvidenceV1.validate":          "4777e91b940ec2554a1c70e4ea93b197d04a33e978236923200dadf2321dd305",
			"wireWrapRecordedV1.validate":                  "3c6676c2943d40de5827fe8325f47ae1d80a76a62aa54d14fa4767ae6b9cf3f5",
		},
	}
}

func digestQualifiedFunctions(t *testing.T, path string) map[string]string {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	digests := make(map[string]string)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := function.Name.Name
		if function.Recv != nil && len(function.Recv.List) == 1 {
			name = receiverName(function.Recv.List[0].Type) + "." + name
		}
		var formatted bytes.Buffer
		if err := format.Node(&formatted, fileSet, function); err != nil {
			t.Fatalf("format %s.%s: %v", path, name, err)
		}
		if _, duplicate := digests[name]; duplicate {
			t.Fatalf("%s has multiple functions or methods named %s", path, name)
		}
		digests[name] = fmt.Sprintf("%x", sha256.Sum256(formatted.Bytes()))
	}
	return digests
}

func sortedMapKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
