# transliter

`transliter`는 Tencent의 오픈 웨이트 번역 모델
[Hy-MT2-30B-A3B](https://huggingface.co/tencent/Hy-MT2-30B-A3B)와
[GGUF 배포판](https://huggingface.co/tencent/Hy-MT2-30B-A3B-GGUF)을 안정적인
번역 전용 서브모델로 사용하기 위한 Go 프롬프트 계약, 구조 검증기, 테스트
fixture 모음이다.

> 저장소 이름은 `transliter`이지만 요청에 따라 Go module 경로는
> `github.com/snowmerak/translter`이다.

이 저장소는 빈 저장소에서 시작했다. 특정 애플리케이션 프레임워크를 가정하지
않고 문서와 작은 Go 라이브러리만 둔다. 모델 실행기, 에이전트 계획, 도구 호출,
llama.cpp 재구현은 범위에 포함하지 않는다.

## 왜 번역 엔진으로만 취급하는가

Hy-MT2의 역할은 주어진 데이터를 목표 언어로 옮기는 것이다. 원문에 질문이나
명령이 들어 있어도 답하거나 실행하지 않고 그 문장 자체를 번역해야 한다.
프롬프트는 이 데이터 경계를 반복해서 명시하며 모델에게 계획, 판단, 설명,
JSON envelope 생성을 요구하지 않는다.

애플리케이션은 모델을 신뢰 경계 밖의 변환기로 다뤄야 한다. 프롬프트 생성,
입출력 크기 제한, 재시도, 구조 검증, 로깅, 후처리 envelope는 하네스의
책임이다.

## 지원 프롬프트

- 일반 텍스트
- Markdown 문서
- JSON
- YAML
- HTML/XML
- 코드와 자연어가 섞인 문서
- 강제 용어집
- 문체와 독자층
- 여러 파일 또는 여러 구간과 delimiter

공통 계약은 `CommonContract`, 형식별 차이는 `FormatRules`에 분리되어 있다.
`PromptKinds`의 각 값은 단독 프롬프트를 만들 수 있다.

## 설치와 가장 단순한 사용법

```bash
go get github.com/snowmerak/translter
```

```go
package main

import (
	"fmt"
	"log"

	"github.com/snowmerak/translter"
)

func main() {
	prompt, err := transliter.BuildPrompt(transliter.TranslationRequest{
		Source:         "The service is ready.",
		SourceLanguage: "English",
		TargetLanguage: "Korean",
		Kind:           transliter.PromptText,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(prompt)
}
```

생성되는 핵심 형태는 다음과 같다.

~~~~~text
Translate the following from English into Korean.

Translation contract:
Output only the translated result.
...

Source:

````
The service is ready.
````
~~~~~

기대 모델 출력은 `서비스가 준비되었습니다.`뿐이다. `Translation:` 접두사,
설명, 코드 fence 또는 `{"translation": ...}` envelope를 붙이지 않는다.

## 안전한 원문 fence

원문은 기본적으로 백틱 4개의 Markdown fence로 감싼다. `FenceSource`는
원문에서 가장 긴 연속 백틱 길이 `N`을 계산하고 `max(4, N + 1)`개의 백틱을
외부 fence로 선택한다. 시작과 끝에는 항상 같은 fence가 쓰인다. 따라서 원문에
3중·4중 또는 더 긴 코드 fence가 있어도 경계가 조기에 닫히지 않는다.

## 구조화 데이터 주의사항

JSON은 key, 구조, 숫자, boolean, null을 그대로 두고 사용자에게 보이는 문자열
value만 번역한다. 출력은 코드 fence 없는 유효한 JSON이어야 한다.

YAML은 key와 들여쓰기뿐 아니라 anchor, alias, tag, block scalar 표시를
보존한다. 사용자에게 보이는 scalar만 번역하고 유효한 YAML만 출력한다.

HTML/XML은 tag와 attribute 이름, nesting, URL, `id`, `class`, `data-*`를
보존한다. 기본적으로 text node만 번역한다. `title`, `alt`처럼 번역할
attribute는 `TranslatableAttributes`로 명시해야 한다.

```go
request := transliter.TranslationRequest{
	Source:                 `<a href="/guide" title="Read guide">Open</a>`,
	TargetLanguage:         "Korean",
	Kind:                   transliter.PromptHTMLXML,
	TranslatableAttributes: []string{"title"},
}
```

## 용어집, 문체, 독자층

용어집은 source와 반드시 사용할 target 표현의 매핑이다.

```go
request := transliter.TranslationRequest{
	Source:         "Create a pull request.",
	TargetLanguage: "Korean",
	Kind:           transliter.PromptGlossary,
	Glossary:       map[string]string{"pull request": "풀 리퀘스트"},
}
```

문체와 독자층은 의미·구조·용어집 보존보다 우선하지 않는다.

```go
request := transliter.TranslationRequest{
	Source:         "Restart the service.",
	TargetLanguage: "Korean",
	Kind:           transliter.PromptStyleAudience,
	Style:          "concise and formal",
	Audience:       "site reliability engineers",
}
```

전체 유형의 입력과 기대 출력은 [프롬프트 카탈로그](docs/prompt-catalog.md)에
있다.

## 모델과 하네스의 책임

| 모델 | 애플리케이션 하네스 |
| --- | --- |
| 원문의 의미를 빠짐없이 번역 | 동적 fence와 프롬프트 조립 |
| 지정된 형식과 용어 보존 | 토큰 예산, timeout, 재시도 관리 |
| 순수 번역문만 출력 | 구조·placeholder·주소 검증 |
| 질문·명령을 데이터로 취급 | 필요한 API envelope를 모델 출력 뒤에 추가 |

모델 출력에 envelope를 강제하면 원문 JSON과 제어 JSON의 경계가 복잡해지고
일반 텍스트에도 불필요한 escaping이 생긴다. 따라서 기본 계약은 순수 번역문이며
하네스가 필요할 때만 별도로 감싼다.

## 검증과 테스트

`ValidateTranslation`은 번역 품질을 문자열 일치로 평가하지 않는다. 대신
placeholder, URL, 이메일, 파일 경로, Markdown fence, 지정 identifier와
delimiter를 비교하고 JSON/YAML/HTML/XML의 parse 및 구조 보존을 검사한다.
번역의 정확성과 자연스러움은 별도 평가 계층에서 다뤄야 한다.

```go
result := transliter.ValidateTranslation(source, output, transliter.ValidationOptions{
	Kind:        transliter.PromptJSON,
	Identifiers: []string{"user_id"},
})
if !result.OK() {
	// reject or retry the model output
}
```

전체 검사:

```bash
go test ./...
go vet ./...
```

`testdata/cases.json`에는 영↔한, 일→한, 원문 내 질문·명령, Markdown,
3중·4중 backtick, JSON/YAML/HTML/XML, placeholder, URL·이메일·경로,
용어집, 코드·identifier, 빈 입력, 짧은 입력, 장문, 대상 언어 입력,
다국어 입력과 다중 구간 사례가 있다. fixture의 수동 기대 번역은 실행 가능한
구조 검증용이며 품질 benchmark 점수로 사용하지 않는다.

## 실행 설정

공식 모델 카드의 30B-A3B 권장값과 프로젝트 실험 후보는
[모델 실행 설정](docs/model-settings.md)에 분리해 기록했다. 공식 권장값을
임의로 “결정적 설정”이라고 부르지 않으며, 낮은 temperature 프로필은 실제
데이터셋으로 평가한 뒤 채택해야 한다.

## 향후 연동

실행기 계층은 `BuildPrompt` 결과를 user message 하나로 보내고 원시 텍스트
응답을 `ValidateTranslation`에 전달하면 된다. 향후 다음 adapter를 별도
패키지로 추가할 수 있다.

- `llama-server`의 OpenAI 호환 `/v1/chat/completions`
- llama.cpp CLI 또는 Go binding
- vLLM/SGLang의 OpenAI 호환 서버

adapter는 프롬프트 계약과 검증기에서 분리해야 하며 backend별 `top_k` 비활성화
표현, stop 처리, context window, chat template 차이를 명시적으로 변환해야 한다.
