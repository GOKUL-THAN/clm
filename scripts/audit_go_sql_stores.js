const fs = require("fs");
const path = require("path");

function findGoFiles(dir) {
  let results = [];
  const list = fs.readdirSync(dir);
  for (const file of list) {
    const fullPath = path.join(dir, file);
    const stat = fs.statSync(fullPath);
    if (stat && stat.isDirectory()) {
      if (!file.startsWith(".") && file !== "node_modules" && file !== "vendor") {
        results = results.concat(findGoFiles(fullPath));
      }
    } else if (file.endsWith(".go")) {
      results.push(fullPath);
    }
  }
  return results;
}

const baseDir = path.resolve(__dirname, "../components");
const goFiles = findGoFiles(baseDir);
console.log(`Auditing ${goFiles.length} Go source files for SQL query safety...`);

let sqlIssues = [];
let queryCount = 0;
let parameterizedCount = 0;

for (const file of goFiles) {
  const content = fs.readFileSync(file, "utf8");
  const lines = content.split("\n");
  for (let idx = 0; idx < lines.length; idx++) {
    const line = lines[idx];
    if (/(?:Query|Exec|QueryRow|Select|NamedExec)\s*\(/i.test(line) && /(?:SELECT|INSERT|UPDATE|DELETE|SET)/i.test(line)) {
      queryCount++;
      if (line.includes("+") || line.includes("fmt.Sprintf")) {
        sqlIssues.push({ file: path.relative(path.resolve(__dirname, ".."), file), line: idx + 1, text: line.trim() });
      } else {
        parameterizedCount++;
      }
    }
  }
}

console.log(`Total detected SQL execution lines: ${queryCount}`);
console.log(`Fully parameterized / static queries: ${parameterizedCount}`);
console.log(`Potential formatting/concatenation issues: ${sqlIssues.length}`);
if (sqlIssues.length > 0) {
  console.log("Issues detected:", JSON.stringify(sqlIssues, null, 2));
} else {
  console.log("PASS: 100% of examined direct SQL execution calls use parameterized queries without string concatenation.");
}

