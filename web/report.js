function matches(text,query){return text.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())}
if(typeof module!=='undefined')module.exports={matches};
if(typeof document!=='undefined'){
const input=document.getElementById('search'),articles=[...document.querySelectorAll('article')];
function filter(){let visible=0;for(const article of articles){article.hidden=!matches(article.textContent,input.value);if(!article.hidden)visible++}document.getElementById('count').textContent=`${visible} / ${articles.length} articles`}
input.addEventListener('input',filter);filter();
}
