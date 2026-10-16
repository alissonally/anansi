# Etapa 5 — Exportação e importação

**Status:** concluída.

## Tarefas em segundo plano

Operações longas rodam como tarefas (`internal/tasks`): progresso por evento `task:update` (no máximo a cada
200 ms), cancelamento, contagem de avisos e resultado final. O painel **Tarefas** (barra de status) mostra
barra de progresso, tempo, mensagem atual, cancelar, abrir a pasta do arquivo gerado e limpar concluídas;
ao terminar aparece um aviso. As tarefas são canceladas no encerramento do app.

## Exportar linhas (HeidiSQL: "Export grid rows")

Botão **Exportar** na aba de dados e no painel de resultados, e "Copiar → Em outro formato…" no menu da grade.

- **Formatos** (`internal/gridexport`, uma única implementação em Go): CSV (separador, delimitador, fim de
  linha e texto do NULL configuráveis), CSV para Excel (`;`, BOM UTF-8, CRLF), TSV, JSON (com nome da tabela e
  consulta), JSON Lines, XML, HTML (documento com estilo), Markdown (colunas numéricas alinhadas à direita),
  LaTeX, Wiki Textile e Jira, array PHP e SQL (INSERT, INSERT IGNORE, REPLACE, DELETE+INSERT, UPDATE, com
  INSERT multi-linha e opção de omitir AUTO_INCREMENT). Inteiros grandes e DECIMAL mantêm a precisão em JSON.
- **Linhas**: selecionadas, carregadas na grade ou **todas** — **melhoria**: o HeidiSQL exporta só o que está
  carregado; aqui "todas" reexecuta a consulta (ou a tabela com o filtro, a ordenação e as colunas atuais) e
  envia as linhas direto do servidor para o arquivo, em streaming, sem limite e sem truncar valores.
- **Destino**: arquivo (diálogo nativo) ou área de transferência. As opções são lembradas.
- Nos formatos SQL a chave da tabela (quando conhecida) identifica as linhas de UPDATE/DELETE.

## Exportar como SQL (HeidiSQL: "Export database as SQL")

Menu da árvore (conexão, database, tabela), aba do database (objetos selecionados) e aba do servidor
(databases selecionados).

- Seleção de databases e, por database, tabelas/views, procedures e functions, triggers e eventos.
- Opções: DROP/CREATE DATABASE, DROP/CREATE dos objetos, remover `AUTO_INCREMENT=n`, remover `DEFINER`, dados
  (nenhum, INSERT, INSERT IGNORE, REPLACE, DELETE+INSERT), tamanho máximo de cada INSERT multi-linha,
  transação por tabela, comentários.
- **Destinos**: arquivo `.sql`, `.sql.gz` (compactado), **pasta** com um arquivo por objeto (cada um com o
  cabeçalho de sessão, executável sozinho) ou **outro database** da mesma ou de outra conexão aberta
  (executa as instruções; somente leitura recusado, produção pede confirmação; "continuar após erros").
- Mostra o **comando mysqldump equivalente**.

Como o dump é feito (`internal/dump`):

- Leitura num **snapshot consistente** (`REPEATABLE READ` + `START TRANSACTION WITH CONSISTENT SNAPSHOT`, como
  `mysqldump --single-transaction`) e `TIME_ZONE='+00:00'`.
- Cabeçalho/rodapé de sessão iguais aos do mysqldump/HeidiSQL (charset, time zone, `FOREIGN_KEY_CHECKS=0`,
  `SQL_MODE='NO_AUTO_VALUE_ON_ZERO'`, `SQL_NOTES=0`), restaurados no final.
- Colunas geradas não entram nos INSERTs; binários como `_binary 0x…`; textos com os escapes do mysqldump.
- Views: primeiro uma tabela provisória com as colunas, no fim a view real — views que dependem de outras
  funcionam em qualquer ordem. Rotinas e eventos com `DELIMITER ;;`; triggers por último (depois dos dados,
  para não dispararem na carga) com o `sql_mode` original.
- **Correções em relação ao HeidiSQL** ao copiar para outro database: o nome da view deixa de ser qualificado
  com o database de origem e as referências da definição passam a apontar para o destino; o `CREATE TRIGGER`
  é reconstruído a partir do catálogo, porque o MariaDB devolve no `SHOW CREATE TRIGGER` o texto original
  qualificado com o database de origem (a cópia criaria o trigger no lugar errado — e ficava bloqueada pelo
  próprio snapshot).

## Executar arquivo SQL

"Executar arquivo SQL…" na árvore e nas abas de database/servidor: escolhe o arquivo (`.sql` ou `.sql.gz`),
confirma (com "parar no primeiro erro") e executa em segundo plano **sem abrir no editor**, lendo em blocos de
4 MiB (`internal/sqlfile`). O divisor de instruções ganhou um modo incremental (`sqllex.SplitPartial`) que
preserva o `DELIMITER` atual entre blocos; o teste alimenta o mesmo script em pedaços de 1 byte a 1000 bytes e
confere que as instruções são idênticas. Perfis somente leitura só aceitam leitura e instruções de sessão.

## Importar CSV (HeidiSQL: "Import CSV file")

- **Detecção** (`csvimport.Detect`): separador (`,` `;` tab `|` pela consistência do número de campos),
  escape, cabeçalho e **tipos** por coluna (TINYINT…BIGINT com UNSIGNED, DECIMAL(p,s), DOUBLE, DATE, DATETIME,
  VARCHAR arredondado de 10 em 10, TEXT/MEDIUMTEXT), anulabilidade.
- Prévia das primeiras linhas atualizada ao mudar separador, delimitador, escape (`""` ou `\`), codificação
  (UTF-8 ou Latin-1/Windows-1252), linhas ignoradas, marcador de NULL e "vazio = NULL".
- Destino: tabela existente (mapeamento coluna do arquivo → coluna da tabela ou ignorar) ou **tabela nova**
  criada com os tipos detectados (via editor de tabela do backend).
- Opções: duplicadas (erro, IGNORE, REPLACE), esvaziar antes, **tudo ou nada** (uma transação), números no
  formato local (separadores de milhar e decimal, só nas colunas numéricas).
- Dois métodos, como no HeidiSQL: o **aplicativo lê o arquivo** (padrão; parser próprio em streaming e
  INSERTs multi-linha com **valores vinculados**, respeitando o limite de 65.535 parâmetros) ou o **servidor lê**
  (`LOAD DATA LOCAL INFILE`, exige `local_infile=ON`; o driver libera apenas aquele arquivo — `allowAllFiles`
  continua desligado).

## Arquivos em células

No editor de célula: **Carregar arquivo…** (até 16 MiB, para colunas binárias; HeidiSQL "Insert files") e
**Salvar em arquivo…** (binário ou texto; HeidiSQL "Save BLOB to file").

## Correções encontradas no caminho

- **Comentários executáveis** `/*! … */` e `/*M! … */` eram tratados como comentário: uma instrução feita só
  deles era descartada pelo divisor (o cabeçalho de qualquer dump do mysqldump não era executado no editor).
  Agora são tokens significativos e a classificação usa o conteúdo — inclusive para as políticas: um
  `/*!50000 DROP TABLE t */` é DDL destrutivo.
- O `Split` tinha custo quadrático (número de linha recontado desde o início para cada instrução): 8,5× mais
  rápido em 4 MB, o que também acelera scripts grandes no editor.
- O pacote de formatos se chama `gridexport` porque `export` gera um `namespace export` inválido nos bindings
  TypeScript do Wails.

## Testes

- Unitários: `gridexport` (todos os formatos, escapes, NULL, binários, inteiros grandes, BOM do Excel, SQL
  multi-linha, DELETE+INSERT e UPDATE pela chave), `sqllex` (comentários executáveis, divisão incremental),
  `sqlfile` (script maior que um bloco com UTF-8 e DELIMITER cruzando blocos, erros com e sem parada),
  `csvimport` (RFC 4180, estilo MySQL com `\`, tab, Latin-1, BOM, detecção de layout e tipos).
- Integração (`internal/integration/transfer_test.go`, MySQL 8.4 e MariaDB 11.4): database com FK, coluna
  gerada, BIT, DECIMAL, DATETIME(6), JSON, MEDIUMTEXT de 70 KB, BLOB, texto com aspas, barra, quebra de linha e
  emoji, views dependentes fora de ordem, procedure, function, trigger e evento → **dump em texto restaurado
  pelo executor de arquivos** e **cópia direta para outro database**: `CHECKSUM TABLE` idêntico, views,
  rotinas, triggers e eventos recriados, AUTO_INCREMENT preservado. Importação CSV com aspas, separador
  dentro do texto, número local, NULL e quebra de linha; importação atômica revertida em erro.
- No app: exportação para a área de transferência em Markdown e cópia de `demo_shop` para `demo_copy` pelo
  diálogo, acompanhada no painel de tarefas.

## Limitações

- Diálogos de arquivo são nativos (GTK); o caminho escolhido é o único que o backend aceita ler/escrever.
- Ao copiar rotinas e triggers para outro database, referências qualificadas ao database de origem dentro dos
  corpos não são reescritas (só nas views).
