# Plano Aceito: Release/CHANGELOG e Dependências

## Resumo

- Corrigir a origem do desalinhamento entre GitHub Release e `CHANGELOG.md` no `releasepr`.
- Publicar uma nova versão do `releasepr` e atualizar `compozy/compozy` para consumi-la.
- Atualizar dependências e toolchain com foco em segurança e dependências diretas.

## Mudanças principais

- No `releasepr`, gerar artefatos de release com a versão alvo informada, não com `unreleased`.
- Corrigir os templates `git-cliff` para preservar quebra de linha nas listas e gerar links corretos.
- Atualizar dependências Go vulneráveis/diretas e alinhar workflows para Go patch atual.
- No `compozy/compozy`, atualizar `PR_RELEASE_MODULE`, corrigir `cliff.toml` e validar o fluxo de release.

## Testes

- Adicionar regressões para argumentos do `git-cliff` e para o conteúdo versionado de `CHANGELOG.md`.
- Rodar `make lint` e `make test` em `releasepr`.
- Rodar validações do repo consumidor após a atualização.

## Assumptions

- O checkout local de `compozy/compozy` pode estar atrás do remoto; conferir antes de editar.
- Um novo `## Unreleased` só deve conter commits posteriores à release, não o conteúdo já publicado.
