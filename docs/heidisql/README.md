# Paridade com o HeidiSQL — plano e registro das etapas

Este diretório documenta o trabalho de trazer para o Anansi as funcionalidades do
[HeidiSQL](https://www.heidisql.com/) (código-fonte lido em `~/dev/HeidiSQL/source`, Delphi), adaptando-as
à arquitetura Wails + Go + Vue do projeto e melhorando o que fosse possível.

- [`inventario.md`](inventario.md) — inventário das funcionalidades do HeidiSQL, com o status de cada uma no
  Anansi.
- `etapa-N-*.md` — registro de cada etapa: o que foi feito, decisões, SQL gerado, testes e pendências.

## Princípios

1. **Segurança antes de paridade.** Toda instrução gerada pelo app passa pelas mesmas políticas das
   consultas do editor: conexões somente leitura bloqueiam escrita, produção pede confirmação e operações
   destrutivas pedem confirmação. O frontend envia **especificações estruturadas**; o SQL é montado e
   validado no backend (quoting de identificadores e literais).
2. **Nada de travar a UI.** Operações longas (dump, importação, busca em todas as tabelas, manutenção) rodam
   em tarefas cancelaveis com progresso por eventos.
3. **Engine-neutro onde for barato.** As capacidades novas são interfaces opcionais em
   `internal/database`; a implementação MySQL/MariaDB fica em `internal/database/mysql`.
4. **Testes reais.** Cada etapa acrescenta testes unitários e de integração contra MySQL 8.4 e MariaDB 11.4.

## Etapas

| # | Etapa | Conteúdo | Status |
|---|---|---|---|
| 1 | Fundação de objetos e servidor | Abas de objeto (não só SQL), log de SQL, metadados completos (tamanhos, FKs, checks, triggers, events, rotinas, DDL), aba do servidor (databases, variáveis, status, processos, estatísticas de comandos), aba do database (lista de objetos), árvore completa com ações (drop, truncate, renomear, copiar DDL) | [concluída](etapa-1-objetos-e-servidor.md) |
| 2 | Dados da tabela | Grid editável (UPDATE/INSERT/DELETE com detecção de chave), filtros WHERE e rápidos, ordenação, seleção de colunas, paginação, editores de célula (texto, hex, JSON, ENUM/SET, data), valor completo de células truncadas, lookup de chave estrangeira | [concluída](etapa-2-dados-da-tabela.md) |
| 3 | Editores de objetos | Editor de tabela com ALTER TABLE (colunas, índices, FKs, checks, partições, opções), views, procedures/functions, triggers e events | [concluída](etapa-3-editores-de-objetos.md) |
| 4 | Editor de consultas | Painel de ajuda (colunas, funções, palavras-chave, snippets), parâmetros de consulta, EXPLAIN, profiling, resultados editáveis, executar arquivo SQL grande | [concluída](etapa-4-editor-de-consultas.md) |
| 5 | Exportação e importação | Exportar grid (CSV, TSV, JSON, JSONL, XML, HTML, Markdown, LaTeX, Wiki, PHP, SQL INSERT/REPLACE/UPSERT), dump SQL de databases/tabelas, importação CSV e de arquivos SQL | [concluída](etapa-5-exportacao-e-importacao.md) |
| 6 | Ferramentas | Manutenção (CHECK/ANALYZE/OPTIMIZE/REPAIR/CHECKSUM), busca de texto no database, edição em massa de tabelas, cópia de tabela, gerenciador de usuários e privilégios, troca de senha, comparação de estruturas | [concluída](etapa-6-ferramentas.md) |
| 7 | Sessões | Túnel SSH, socket Unix, script de inicialização, keep-alive, pastas, comentários, exportar/importar configurações | [concluída](etapa-7-sessoes.md) |
| 8 | Preferências e acabamento | Preferências do grid e do editor, atalhos, documentação final | [concluída](etapa-8-preferencias-e-acabamento.md) |

O status atualizado de cada item fica no [inventário](inventario.md).
