# Etapa 3 — Editores de objetos

**Status:** concluída.

Todos os objetos do database têm editor estruturado, como no HeidiSQL: tabela, view, procedure/function,
trigger e evento. "Novo(a) …" na árvore e na aba do database abre o editor numa aba (o antigo diálogo de
criação de tabela foi substituído pelo editor completo, que também cobre FKs, CHECKs e partições).

## Editor de tabela

Sub-abas **Colunas**, **Índices**, **Chaves estrangeiras**, **CHECK**, **Opções**, **Particionamento** e
**ALTER/CREATE** (o SQL que será executado, atualizado enquanto se edita).

- **Colunas**: nome, tipo, tamanho/valores, UNSIGNED, ZEROFILL, NULL, padrão (sem padrão, NULL, valor,
  expressão, AUTO_INCREMENT), ON UPDATE, collation, coluna gerada (expressão + VIRTUAL/STORED — PERSISTENT no
  MariaDB), INVISIBLE e comentário. Adicionar copia tipo/tamanho da coluna selecionada (como o HeidiSQL),
  mover para cima/baixo, remover (limpa as partes de índice que a usavam), renomear atualiza índices e FKs.
  Trocar o tipo redefine o tamanho para o usual do tipo e desmarca atributos que não se aplicam.
  Menu de contexto: criar PRIMARY/UNIQUE/KEY/FULLTEXT/SPATIAL com a coluna ou adicioná-la a um índice.
- **Índices**: tipo, nome (automático se vazio), algoritmo, comentário, invisível (`INVISIBLE` no MySQL,
  `IGNORED` no MariaDB), partes com prefixo, `DESC` e partes funcionais (expressão, MySQL 8.0.13+).
- **Chaves estrangeiras**: colunas locais, database e tabela referenciadas (listas do servidor), colunas
  referenciadas e ações ON UPDATE/ON DELETE.
- **CHECK**: nome e expressão.
- **Opções**: engine, collation (com "converter os dados existentes" → `CONVERT TO CHARACTER SET`), formato de
  linha, próximo AUTO_INCREMENT, AVG_ROW_LENGTH, MAX_ROWS, CHECKSUM.
- **Particionamento**: cláusula `PARTITION BY …` livre. **Melhoria**: o HeidiSQL só usa partições no CREATE;
  aqui alterar a cláusula gera o ALTER correspondente e apagá-la gera `REMOVE PARTITIONING`.

### Como o ALTER é gerado (`internal/database/mysql/tabledef.go`)

O backend **recarrega a definição atual do servidor** e calcula a diferença — não confia na cópia do
"original" que o frontend tem. Se uma coluna/índice/FK editado não existe mais (alterado por outra sessão), o
salvamento é recusado com "recarregue o editor". Ordem, seguindo o HeidiSQL:

1. FKs alteradas ou removidas: `ALTER TABLE … DROP FOREIGN KEY` em instruções separadas (o MySQL não permite
   remover e recriar a mesma FK num só ALTER).
2. Um único `ALTER TABLE` com, nesta ordem: remoção de índices e CHECKs, `CHANGE COLUMN` (renomear, alterar
   e reposicionar numa cláusula só) e `ADD COLUMN … AFTER/FIRST`, `DROP COLUMN`, novos índices, novas FKs,
   novos CHECKs, opções da tabela e particionamento.
3. Mudança só de visibilidade de índice: `ALTER INDEX … VISIBLE|INVISIBLE` (ou `IGNORED|NOT IGNORED`).
4. Renomear a tabela: `RENAME TABLE`.

Uma coluna só recebe `FIRST/AFTER` quando a posição relativa entre as colunas que já existiam muda (colunas
novas não "empurram" as antigas para um CHANGE desnecessário).

A leitura usa o `information_schema` (COLUMNS, STATISTICS, KEY_COLUMN_USAGE/REFERENTIAL_CONSTRAINTS,
CHECK_CONSTRAINTS, TABLES) e trata as diferenças entre os servidores:

- **Padrões**: no MySQL 8 `EXTRA = DEFAULT_GENERATED` indica expressão; no MariaDB literais vêm entre aspas,
  `NULL` vem como texto e o resto é expressão. Expressões que não são funções de data são escritas como
  `DEFAULT (expr)`, exigido pelo MySQL 8.0.13+.
- Colunas geradas, INVISIBLE, ON UPDATE e SRID (MySQL) vêm do `EXTRA`/`GENERATION_EXPRESSION`/`SRS_ID`;
  índices funcionais e visibilidade vêm de `EXPRESSION`/`IS_VISIBLE` (MySQL) ou `IGNORED` (MariaDB).
- Particionamento é lido do `SHOW CREATE TABLE` (o MariaDB não marca tabelas particionadas em
  `CREATE_OPTIONS`).

**Teste de idempotência**: carregar uma tabela com todos esses recursos e salvá-la sem mudanças gera **zero**
instruções nos dois servidores — a garantia de que o editor não faz ALTERs fantasmas.

Expressões digitadas (padrões, colunas geradas, CHECK, índices funcionais, partições) entram como SQL, mas são
validadas: parênteses balanceados, aspas fechadas e nada de `;` fora de strings. Nomes, tipos, tamanhos,
valores de ENUM/SET, ações de FK, engines e collations são validados e citados pelo backend.

## Editores de view, rotina, trigger e evento

Todos compartilham o mesmo casco: campos no topo, corpo num editor Monaco, aba **SQL gerado**, Salvar
(Ctrl+S) e Descartar, erro com a instrução que falhou.

| Editor | Campos | Como salva |
|---|---|---|
| View | nome, algoritmo, SQL SECURITY, CHECK OPTION, definer, SELECT (formatado ao abrir) | `CREATE`/`ALTER VIEW` + `RENAME TABLE` se renomeada |
| Procedure/Function | nome, tipo, retorno, acesso a dados, SQL SECURITY, definer, DETERMINISTIC, comentário, parâmetros (modo, nome, tipo; reordenáveis), corpo | substituição segura (abaixo) |
| Trigger | nome, tabela, momento, evento, definer, corpo | substituição segura |
| Evento | nome, estado (ativado/desativado/desativado na réplica), agenda (uma vez em… / a cada N unidade com início e fim), ON COMPLETION PRESERVE, definer, comentário, corpo | `CREATE EVENT` ou `ALTER EVENT … RENAME TO` |

O campo **definer** sugere as contas de `mysql.user` (quando o usuário pode lê-las).

### Substituição segura de rotinas e triggers

O servidor não altera o corpo dessas rotinas: é preciso recriá-las. O HeidiSQL cria uma cópia temporária para
validar a sintaxe; aqui o fluxo completo é:

1. criar a nova versão com nome temporário (`<nome>_dbstudio_tmp`) — se falhar, nada foi tocado;
2. remover a cópia temporária;
3. remover a versão atual e criar a nova;
4. se a criação final falhar, **restaurar a versão anterior** a partir do seu `SHOW CREATE` (o HeidiSQL faz
   isso só para triggers).

Fora de produção esse fluxo não pede a confirmação de "instrução destrutiva" (não há perda de dados); em
produção pede, como qualquer DDL. O editor avisa que privilégios `EXECUTE` da rotina precisam ser refeitos.

## Testes

- Unitários (`internal/database/mysql/tabledef_test.go`): parsing de `COLUMN_TYPE` e listas ENUM com aspas e
  parênteses, padrões do MariaDB, SQL de coluna (collation, expressões, ON UPDATE, gerada no MariaDB),
  rejeição de expressões com `;`/parênteses abertos e de tamanhos que tentam injetar SQL, diff de posições
  com o mínimo de `CHANGE` e coluna inexistente no servidor.
- Integração (`internal/integration/editors_test.go`, MySQL 8.4 e MariaDB 11.4, pela API real):
  - tabela com ENUM com aspas, DECIMAL, coluna gerada STORED, DATETIME(3) com `CURRENT_TIMESTAMP(3)` e
    ON UPDATE, TEXT, coluna INVISIBLE, collation de coluna, comentário com aspas, índice com prefixo e DESC,
    FULLTEXT, FK com ações, CHECK e opções; **round trip sem alterações = 0 instruções**;
  - edição com renomear + trocar tipo, mover coluna, adicionar e remover colunas, trocar índices, alterar FK,
    trocar CHECK, collation e comentário da tabela, renomear a tabela — e a ordem final das colunas conferida;
  - índice invisível; particionamento criado, lido, round trip e removido;
  - `DROP COLUMN` sem confirmação pede confirmação;
  - function substituída com corpo novo (resultado conferido), **substituição com erro mantém a versão
    original funcionando**, procedure com parâmetro OUT renomeada, trigger substituído (efeito conferido),
    evento criado e alterado com renomeação e nova agenda, view criada e alterada com renomeação, definers.
- No app (Chrome headless): estrutura de tabela, inclusão de coluna com o ALTER mínimo exibido, salvamento,
  índices, FKs e editor de procedure nova.
