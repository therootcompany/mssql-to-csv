-- Two Things:
--
--   1. Create the database before running the script
--      CREATE DATABASE TestDB;
--
--   2. Then run this command to create the fixtures:
--      sqlcmd -S localhost -U 'SA' -i ./utils/fixtures.sql

USE TestDB;

-- Note: these fields are intentionall in the same order as the spreadsheet so
--       that we can easily edit them and their values with only a little regex
CREATE TABLE [TestTable42]
	( [FruitName] NVARCHAR(255)
	, [FruitType] NVARCHAR(255)
	, [FruitQuantity] INT
	, [CreatedAt] DATETIME
	, [UpdatedAt] DATETIME
	)
;

INSERT INTO [TestTable42]
	VALUES
	( 'Orange'
	, 'Citrus'
	, 11
	, CURRENT_TIMESTAMP
	, CURRENT_TIMESTAMP
	)
,
    ( 'Apple'
	, 'Normal'
	, 37
	, CURRENT_TIMESTAMP
	, CURRENT_TIMESTAMP
	)
,
    ( 'Avocado'
	, 'Drupe'
	, 42
	, '2021-02-27T13:59:59Z'
	, CURRENT_TIMESTAMP
	)
;
