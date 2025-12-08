parser grammar ZeonicaParser;

options {
    tokenVocab = ZeonicaLexer;
}

compilationUnit
    : peBlock* EOF
    ;

peBlock
    : PE LPAREN DECIMAL_LITERAL (COMMA DECIMAL_LITERAL)? RPAREN (ENTRY_ARROW loopType)? LBRACE peBody RBRACE
    ;

peBody
    : flatStyle              #FlatBody
    | entryBlock+            #EntryBlockBody
    ;

flatStyle
    : labeledGroup+
    ;

labeledGroup
    : label LBRACE normalInst* RBRACE
    ;

entryBlock
    : ENTRY_BLOCK ENTRY_ARROW loopType LBRACE instGroupList RBRACE
    ;

loopType
    : LOOP
    | ONCE
    ;

instGroupList
    : instGroup*
    ;

instGroup
    : LBRACE normalInst* RBRACE
    ;

label
    : labelID COLON
    ;

labelID
    : IDENTIFIER
    ;

normalInst
    : opCode COMMA operandList RIGHT_ARROW operandList
    | opCode COMMA operandList
    | opCode
    | operand RIGHT_ARROW operand
    ;

operandList
    : operand (COMMA operand)*
    ;

operand
    : predTag LBRACK idList RBRACK
    | IMM LBRACK DECIMAL_LITERAL RBRACK
    | IMM LBRACK OCT_LITERAL RBRACK
    | IMM LBRACK HEX_LITERAL RBRACK
    | IMM LBRACK BINARY_LITERAL RBRACK
    | IMM LBRACK FLOAT_LITERAL RBRACK
    | predTag MEM LBRACK idList RBRACK
    | predTag MEM LBRACK HEX_LITERAL RBRACK
    | labelID
    ;

idList
    : IDENTIFIER (COMMA IDENTIFIER)*
    ;

predTag
    : AND_PRED
    | OR_PRED
    |
    ;

opCode
    : ADD
    | ADDI
    | SUB
    | SUBI
    | MUL
    | DIV
    | MAC
    | INC
    | LLS
    | LRS
    | OR
    | AND
    | XOR
    | NOT
    | FADD
    | FSUB
    | FMUL
    | FDIV
    | FMAC
    | MOV
    | MUL_CONST_ADD
    | NOP
    | CBR
    | CMOV
    | SGE
    ;
