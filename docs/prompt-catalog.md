# 프롬프트 유형별 예제

모든 예제는 Go의 `TranslationRequest`와 `BuildPrompt`로 독립 실행할 수 있다.
아래 “기대 출력”은 envelope나 설명이 없는 모델 원문 응답의 형태다.

## 일반 텍스트

입력:

```text
The service is ready.
```

기대 출력:

```text
서비스가 준비되었습니다.
```

`Kind: PromptText`를 사용한다. 질문이나 명령도 답하거나 실행하지 않고
문장 자체를 번역한다.

## Markdown

입력:

```markdown
# Setup

- Read the [guide](https://example.com).
- Run `install_app`.
```

기대 출력:

```markdown
# 설정

- [가이드](https://example.com)를 읽으세요.
- `install_app`을 실행하세요.
```

`Kind: PromptMarkdown`을 사용한다. link destination, inline code,
fenced code와 fence 길이는 보존한다.

## JSON

입력:

```json
{"title":"Welcome, {{username}}","retry":3,"enabled":true}
```

기대 출력:

```json
{"title":"환영합니다, {{username}}","retry":3,"enabled":true}
```

`Kind: PromptJSON`을 사용한다. key와 비문자열 값은 바뀌지 않으며 모델
출력에는 Markdown fence가 없어야 한다.

## YAML

입력:

```yaml
defaults: &defaults
  retries: 3
  message: Welcome
production:
  <<: *defaults
```

기대 출력:

```yaml
defaults: &defaults
  retries: 3
  message: 환영합니다
production:
  <<: *defaults
```

`Kind: PromptYAML`을 사용한다. key, indentation, anchor, alias, tag,
block scalar 구문을 보존한다.

## HTML/XML

입력:

```html
<a id="guide" href="/docs" title="Read guide">Open</a>
```

`translatable_attributes=("title",)`일 때의 기대 출력:

```html
<a id="guide" href="/docs" title="가이드 읽기">열기</a>
```

`title`을 명시하지 않았다면 그 값도 보존해야 한다. XML 선언이 있는 입력은
well-formed XML 출력으로 검증한다.

## 코드와 자연어 혼합

입력:

````markdown
Call `loadUser(user_id)` to load the user.

```ts
const user = loadUser(user_id);
```
````

기대 출력:

````markdown
사용자를 불러오려면 `loadUser(user_id)`를 호출하세요.

```ts
const user = loadUser(user_id);
```
````

`Kind: PromptMixedCode`를 사용하고 보호할 identifier를 검증기에도
전달한다.

## 용어집

```go
transliter.TranslationRequest{
	Source:         "Create a pull request in the repository.",
	TargetLanguage: "Korean",
	Kind:           transliter.PromptGlossary,
	Glossary: map[string]string{
		"pull request": "풀 리퀘스트",
		"repository":   "저장소",
	},
}
```

기대 출력:

```text
저장소에 풀 리퀘스트를 생성하세요.
```

## 문체와 독자층

```go
transliter.TranslationRequest{
	Source:         "Restart the service now.",
	TargetLanguage: "Korean",
	Kind:           transliter.PromptStyleAudience,
	Style:          "formal operations manual",
	Audience:       "site reliability engineers",
}
```

기대 출력 형태:

```text
서비스를 즉시 재시작하십시오.
```

문체는 의미, 형식, placeholder, 용어집을 바꾸는 권한이 아니다.

## 여러 파일 또는 구간

입력:

```text
<<<FILE:README.md>>>
Welcome.
<<<SECTION:2>>>
Install now.
<<<FILE:guide.md>>>
Read the guide.
```

기대 출력:

```text
<<<FILE:README.md>>>
환영합니다.
<<<SECTION:2>>>
지금 설치하세요.
<<<FILE:guide.md>>>
가이드를 읽으세요.
```

`Kind: PromptSegmented`와 정확한 `Delimiters` 목록을 사용한다. 검증기는
delimiter의 개수뿐 아니라 등장 순서도 비교한다.
