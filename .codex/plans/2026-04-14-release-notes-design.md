# Plano Aceito: Release Notes End-to-End

## Resumo

- Adicionar notas de release autoradas em `.release-notes/*.md` com um novo comando `pr-release add-note`.
- Gerar `RELEASE_NOTES.md` como composição de changelog versionado e notas customizadas.
- Publicar o GitHub Release deste repo a partir desse arquivo, arquivar as notas consumidas por versão e manter rollback correto no fluxo com saga.

## Mudanças principais

- Adicionar um comando Cobra `add-note` com `--title` e `--type` obrigatórios e `--body` opcional.
- Implementar criação, coleta, renderização e arquivamento de release notes em `internal/usecase`.
- Estender o fluxo do orquestrador para gerar `RELEASE_NOTES.md`, mover `.release-notes/*.md` para `.release-notes/archive/vX.Y.Z/` e restaurar esses arquivos em rollback.
- Estender `GitExtendedRepository` com `MoveFile(ctx, from, to)` implementado via `git mv`.
- Corrigir o stage do commit para incluir `RELEASE_NOTES.md` e `.release-notes`.
- Atualizar workflow e dry-run para chamar GoReleaser com `--release-notes=RELEASE_NOTES.md`, com header/footer externos em arquivos Markdown.
- Atualizar README com o fluxo de uso e adoção em repos consumidores.

## Testes

- Testes unitários para criação de notas, slug, template padrão e fallback sem `$EDITOR`.
- Testes unitários para coleta/renderização com ordenação, agrupamento, skip de inválidos e preservação de markdown.
- Testes do PR body para seção opcional de release notes e sanitização segura.
- Testes do orquestrador para geração do arquivo final, arquivamento, rollback e stage correto.
- Testes do repositório Git para `MoveFile`.
- Rodar `make lint` e `make test`.

## Assumptions

- Arquivos de release notes são informativos e não alteram o cálculo de versão.
- `add-note` será um comando de topo.
- Arquivos inválidos não entram no output renderizado, mas serão arquivados após uma release bem-sucedida.
- A implementação cobre o fluxo end-to-end deste repo; repos consumidores seguem por documentação.
