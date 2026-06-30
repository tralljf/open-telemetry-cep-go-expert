# Clima por CEP com Observabilidade

Sistema distribuido em Go com dois microsservicos, OpenTelemetry Collector e Zipkin.

## Servicos

- `service-a`: porta de entrada HTTP. Recebe `POST /weather`, valida o CEP e encaminha para o `service-b`.
- `service-b`: consulta o ViaCEP, consulta a temperatura no Open-Meteo e retorna Celsius, Fahrenheit e Kelvin.
- `otel-collector`: recebe traces OTLP dos servicos e exporta para o Zipkin.
- `zipkin`: interface para visualizar os traces distribuidos.

## Como executar

```bash
docker compose up --build
```

O Servico A ficara disponivel em `http://localhost:8080`.

## Requisicao

```bash
curl -i -X POST http://localhost:8080/weather \
  -H 'Content-Type: application/json' \
  -d '{"cep":"29902555"}'
```

Resposta de sucesso:

```json
{
  "city": "Linhares",
  "temp_C": 28.5,
  "temp_F": 83.3,
  "temp_K": 301.65
}
```

Erros esperados:

- `422 invalid zipcode`: CEP com formato invalido ou campo `cep` que nao seja string.
- `404 can not find zipcode`: CEP valido, mas nao encontrado.

## Zipkin

Acesse:

```text
http://localhost:9411
```

Depois de fazer uma requisicao no Servico A, procure por traces dos servicos `service-a` e `service-b`. O trace deve mostrar o fluxo completo da requisicao e os spans manuais:

- `Busca de CEP`
- `Busca de temperatura`

## Desenvolvimento local

```bash
go test ./...
```

