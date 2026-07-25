# Hy-MT2-30B-A3B 실행 설정

이 문서는 모델 제작자가 공개한 권장값과 이 프로젝트의 실험 후보를 구분한다.
값의 출처가 다르면 같은 표에서 섞어 “공식 권장”으로 부르지 않는다.

## 공식 모델 카드 권장값

[Hy-MT2-30B-A3B 모델 카드](https://huggingface.co/tencent/Hy-MT2-30B-A3B)와
[GGUF 모델 카드](https://huggingface.co/tencent/Hy-MT2-30B-A3B-GGUF)의
Inference and Deployment 절에서 30B-A3B에 다음 값을 권장한다
(확인일: 2026-07-25).

| 항목 | 공식 값 |
| --- | ---: |
| `temperature` | `0.7` |
| `top_p` | `1.0` |
| `top_k` | `-1` |
| `repetition_penalty` | `1.0` |
| `max_tokens` | `4096` |
| 기본 system prompt | 없음 |
| stop sequence | 별도 값이 명시되지 않음 |

모델 카드의 예제는 user message에 번역 프롬프트를 넣고 chat template의
generation prompt를 적용한다. `transformers` 예제에는
`max_new_tokens=4096`가 쓰인다.

## 프로젝트 실험 후보

다음 값은 Tencent의 공식 권장값이 아니다. 동일 입력의 재현성과 형식 이탈
감소를 검증하기 위한 시작점이며, 언어쌍·문서 형식·quantization별 회귀
데이터로 비교해야 한다.

| 프로필 | temperature | top_p | top_k | repetition penalty | maximum output tokens |
| --- | ---: | ---: | ---: | ---: | ---: |
| 공식 기준선 | 0.7 | 1.0 | -1 | 1.0 | 4096 |
| 낮은 변동성 후보 | 0.1 | 1.0 | 비활성화 | 1.0 | 요청별 산정, 상한 4096 |
| greedy 비교군 | 0.0 | 1.0 | 비활성화 | 1.0 | 요청별 산정, 상한 4096 |

주의할 점:

- `top_k=-1`과 `top_k=0` 중 어느 값이 “비활성화”인지는 backend API마다
  다를 수 있다. adapter가 backend 의미를 확인하고 변환해야 한다.
- `temperature=0`도 backend가 완전한 결정성을 보장한다는 뜻은 아니다.
  병렬 연산, quantization, backend 버전이 결과에 영향을 줄 수 있다.
- 출력 상한은 입력 길이만으로 고정 비율을 가정하지 말고 언어쌍과 형식의
  팽창률을 측정해 정한다. 4096은 공식 상한 기준선이지 모든 장문을 수용한다는
  보장이 아니다.
- repetition penalty를 임의로 높이면 반복 억제 과정에서 원문의 중복 문장,
  key, delimiter가 누락될 수 있으므로 기본 비교에서는 `1.0`을 유지한다.

## stop sequence

기본 프롬프트에는 사용자 정의 stop sequence를 넣지 않는다. 원문 자체에
일반적인 구분 문자열이나 fence가 포함될 수 있어 조기 종료 위험이 있기
때문이다. chat template의 EOS와 maximum output tokens를 사용하고, backend가
자동으로 삽입하는 stop token을 기록한다.

사용자 정의 stop이 꼭 필요하면 다음 조건을 모두 만족해야 한다.

1. 원문과 생성 프롬프트에 해당 문자열이 없는지 검사한다.
2. 구조화 데이터의 정상 출력에 나타날 수 없는 값을 사용한다.
3. 잘린 JSON/YAML/HTML/XML을 검증기가 실패로 처리하고 재시도한다.

## 비교 실험

각 프로필을 `testdata/cases.json`의 실제 모델 출력에 적용해 다음을 별도
집계한다.

- 구조 계약 통과율
- 언어쌍별 품질 평가
- glossary 준수율
- 평균 및 최악의 출력 토큰 수
- 동일 입력 반복 실행의 결과 변동
- 원본 가중치와 GGUF quantization 간 차이
