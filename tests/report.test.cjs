const {test}=require('node:test');const assert=require('node:assert/strict');const {matches}=require('../web/report.js');
test('case insensitive',()=>assert.equal(matches('Release Notes','release'),true));
test('trims query',()=>assert.equal(matches('Example','  example '),true));
test('no match',()=>assert.equal(matches('Alpha','Beta'),false));
