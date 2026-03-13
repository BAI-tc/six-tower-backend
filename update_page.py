import sys

file_path = "e:/gamescience/gamesci/src/app/games/[id]/page.js"

with open(file_path, "r", encoding="utf-8") as f:
    content = f.read()

# Chunk 1
old_bg = """      {/* Hero Background */}
      <div className="relative w-full h-[60vh] min-h-[400px] max-h-[700px]">
        {game.background_image && (
          <>
            <img
              src={game.background_image}
              alt={`${game.name} Background`}
              className="w-full h-full object-cover md:object-top"
            />
            {/* Dark gradient fade out to background color */}
            <div className="absolute inset-0 bg-gradient-to-t from-[#1a0a2e] via-[#1a0a2e]/60 to-transparent"></div>
            {/* Cyberpunk accent gradient overlay */}
            <div className="absolute inset-0 bg-gradient-to-r from-[#1a0a2e]/90 via-[#1a0a2e]/30 to-[#2d0a3e]/30 mix-blend-multiply"></div>
          </>
        )}
      </div>"""

new_bg = """      {/* Hero Background */}
      <div className="relative w-full h-[60vh] min-h-[400px] max-h-[700px]">
        {game.background_image && (
          <>
            <img
              src={game.background_image}
              alt={`${game.name} Background`}
              className="w-full h-full object-cover md:object-top"
              style={{ maskImage: 'linear-gradient(to bottom, black 0%, black 50%, transparent 100%)', WebkitMaskImage: 'linear-gradient(to bottom, black 0%, black 50%, transparent 100%)' }}
            />
            {/* Extended Dark gradient fade out to background color to eliminate seam completely */}
            <div className="absolute inset-0 bg-gradient-to-t from-[#1a0a2e] via-[#1a0a2e]/80 to-transparent pointer-events-none"></div>
            {/* Cyberpunk accent gradient overlay */}
            <div className="absolute inset-0 bg-gradient-to-r from-[#1a0a2e]/90 via-[#1a0a2e]/30 to-[#2d0a3e]/30 mix-blend-multiply pointer-events-none"></div>
          </>
        )}
      </div>"""

content = content.replace(old_bg, new_bg)

# Chunk 2
old_genres = """                {/* Genres */}
                {Array.isArray(game.genres) && game.genres.length > 0 && (
                  <div className="flex flex-wrap items-center gap-2 mt-5">
                    {game.genres.map(genre => (
                      <Link
                        href={`/search?name=${genre.name}`}
                        key={genre.id || genre.name}
                        className="text-xs font-semibold bg-white/5 hover:bg-[#ff00ff] hover:text-black border border-white/10 hover:border-[#ff00ff] text-slate-300 px-3.5 py-1.5 rounded-full transition-all duration-300"
                      >
                        {genre.name}
                      </Link>
                    ))}
                  </div>
                )}"""

new_genres = """                {/* 算法评分 & 全球排名 */}
                <div className="flex items-center gap-3 mt-4 text-xs md:text-sm font-bold uppercase tracking-wide">
                   <div className="flex items-center gap-1.5 bg-[#ff00ff]/10 text-[#ff00ff] px-3 py-1.5 rounded border border-[#ff00ff]/30 shadow-[0_0_10px_rgba(255,0,255,0.15)]">
                     <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
                     Algorithm: Exceptional
                   </div>
                   <div className="flex items-center gap-1.5 bg-[#00b0f0]/10 text-[#00b0f0] px-3 py-1.5 rounded border border-[#00b0f0]/30 shadow-[0_0_10px_rgba(0,176,240,0.15)]">
                     <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>
                     Global Rank: #12
                   </div>
                </div>

                {/* Genres */}
                {Array.isArray(game.genres) && game.genres.length > 0 && (
                  <div className="flex flex-wrap items-center gap-2 mt-4">
                    {game.genres.map(genre => (
                      <Link
                        href={`/search?name=${genre.name}`}
                        key={genre.id || genre.name}
                        className="text-xs font-semibold bg-white/5 hover:bg-[#ff00ff] hover:text-black border border-white/10 hover:border-[#ff00ff] text-slate-300 px-3.5 py-1.5 rounded-full transition-all duration-300"
                      >
                        {genre.name}
                      </Link>
                    ))}
                  </div>
                )}

                {/* 推荐理由 */}
                <div className="mt-5 p-4 bg-[#2d0a3e]/40 backdrop-blur-sm rounded-xl border-l-4 border-[#00b0f0] shadow-[0_0_15px_rgba(0,176,240,0.1)]">
                  <div className="text-[#00b0f0] font-bold text-xs uppercase tracking-widest mb-1.5 flex items-center gap-2">
                    <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M12 1L3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5l-9-4zm0 6c1.4 0 2.8 1.1 2.8 2.5V11c.6 0 1.2.6 1.2 1.2v3.5c0 .7-.6 1.2-1.2 1.2H9.2c-.6 0-1.2-.6-1.2-1.2v-3.5c0-.6.6-1.2 1.2-1.2V9.5C9.2 8.1 10.6 7 12 7zm0 1c-.8 0-1.5.7-1.5 1.5V11h3V9.5c0-.8-.7-1.5-1.5-1.5z"/></svg>
                    Why We Recommend
                  </div>
                  <span className="text-slate-200 text-sm leading-relaxed block">
                    A masterpiece that seamlessly blends immersive open-world exploration with cutting-edge visual fidelity. {game.name} pushes the boundaries of its genre, offering an unforgettable experience.
                  </span>
                </div>"""

content = content.replace(old_genres, new_genres)


# Chunk 3
import re
# Regex to match from `            {/* Media Gallery - 直接展示在主界面 */}` down to `            )}` before Similar Games.

lines = content.splitlines()
out_lines = []
in_replace_zone = False
zone_found = False

for line in lines:
    if line.startswith('            {/* Media Gallery - 直接展示在主界面 */}'):
        in_replace_zone = True
        zone_found = True
        
        # Add the new content
        out_lines.append("""            {/* About This Game + Integrated Media */}
            <div className="bg-[#0e141d]/60 backdrop-blur-lg rounded-2xl p-6 md:p-8 border border-white/5 mt-6 shadow-2xl relative overflow-hidden">
              <div className="absolute top-0 right-0 w-64 h-64 bg-[#ff00ff]/5 rounded-full blur-3xl -mr-20 -mt-20 pointer-events-none"></div>
              
              <h2 className="text-xl md:text-2xl font-bold mb-4 text-white flex items-center gap-3 relative z-10">
                <span className="w-1.5 h-6 bg-[#ff00ff] rounded-full inline-block shadow-[0_0_10px_rgba(255,0,255,0.8)]"></span>
                About This Game
              </h2>
              
              <p className="text-slate-300 leading-relaxed text-sm md:text-base relative z-10 whitespace-pre-line">
                {game.description_raw || game.description?.replace(/(<([^>]+)>)/gi, '') || 'No description available for this title.'}
              </p>

              {/* Main Visual Media & Thumbnails underneath */}
              {(screenshots.length > 0 || movies.length > 0) && (
                <div className="mt-8 relative z-10">
                  {/* Main Visual */}
                  <div className="w-full aspect-video rounded-xl overflow-hidden bg-[#000] border border-white/10 mb-3 shadow-[0_0_30px_rgba(0,0,0,0.5)] group relative flex items-center justify-center">
                    {movies.length > 0 ? (
                      <video
                        src={movies[0].data?.['480'] || movies[0].data?.max}
                        poster={movies[0].preview}
                        className="w-full h-full object-cover"
                        controls
                        preload="none"
                      />
                    ) : screenshots.length > 0 ? (
                      <img src={screenshots[0].image} className="w-full h-full object-cover" alt="Main Visual" />
                    ) : null}
                  </div>
                  
                  {/* Thumbnails (4-5) */}
                  <div className="grid grid-cols-4 md:grid-cols-5 gap-2 md:gap-3">
                    {[...movies.map(m=>({type:'video',url:m.preview})), ...screenshots.map(s=>({type:'image',url:s.image}))].slice(1, 6).map((item, i) => (
                      <div key={i} className={`aspect-video rounded-lg overflow-hidden border border-white/10 hover:border-[#ff00ff] transition-all cursor-pointer relative group ${i === 4 ? 'hidden md:block' : ''}`}>
                        <img src={item.url} className="w-full h-full object-cover group-hover:scale-110 group-hover:brightness-110 transition-all duration-500" alt={`Thumbnail ${i+1}`} loading="lazy" />
                        {item.type === 'video' && (
                          <div className="absolute inset-0 flex items-center justify-center bg-black/40 group-hover:bg-black/20 transition-colors">
                            <svg className="w-6 h-6 md:w-8 md:h-8 text-white/90 drop-shadow-lg" fill="currentColor" viewBox="0 0 24 24"><path d="M8 5v14l11-7z" /></svg>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>

            {/* Similar Games Recommended */}
            <div className="mt-8">
              <h2 className="text-xl md:text-2xl font-bold mb-5 text-white flex items-center gap-3">
                <span className="w-1.5 h-6 bg-[#00b0f0] rounded-full inline-block shadow-[0_0_10px_rgba(0,176,240,0.8)]"></span>
                Similar Games
              </h2>
              {game.suggestions_count > 0 ? (
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                  {[1,2,3,4].map((i) => (
                    <div key={i} className="bg-[#0e141d]/80 border border-white/5 rounded-xl overflow-hidden hover:border-[#00b0f0]/50 hover:shadow-[0_0_15px_rgba(0,176,240,0.2)] transition-all cursor-pointer group">
                      <div className="aspect-[16/9] bg-slate-800 relative overflow-hidden">
                        {screenshots[i]?.image ? (
                          <img src={screenshots[i].image} className="w-full h-full object-cover group-hover:scale-110 transition-transform duration-700" alt="Similar Game" />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center text-slate-500 text-xs text-center p-2 bg-[#2d0a3e]/50">Similar Title {i}</div>
                        )}
                        <div className="absolute inset-0 bg-gradient-to-t from-[#0e141d] via-transparent to-transparent opacity-80"></div>
                      </div>
                      <div className="p-3 relative z-10 -mt-6">
                        <div className="text-white font-bold text-sm truncate group-hover:text-[#00b0f0] transition-colors">Similar Title {i}</div>
                        <div className="text-slate-400 text-[10px] mt-1.5 uppercase tracking-wider flex justify-between items-center">
                          <span className="bg-white/10 px-1.5 py-0.5 rounded">Action / RPG</span>
                          <span className="text-[#00b0f0] font-bold">8.{10-i}</span>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="bg-[#2d0a3e]/30 rounded-2xl p-6 border border-[#2d0a3e] text-center text-slate-400">
                  No similar games available.
                </div>
              )}
            </div>

            {/* Cyberpunk Style Comments Section */}
            <div className="mt-8 mb-12">
              <div className="flex items-center justify-between mb-5">
                <h2 className="text-xl md:text-2xl font-bold text-white flex items-center gap-3">
                  <span className="w-1.5 h-6 bg-[#ff00ff] rounded-full inline-block shadow-[0_0_10px_rgba(255,0,255,0.8)]"></span>
                  Neural Network Feedback
                </h2>
                <div className="text-[#ff00ff] text-xs font-mono bg-[#ff00ff]/10 px-2 py-1 rounded border border-[#ff00ff]/30 flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-[#ff00ff] animate-pulse"></span>
                  LIVE SYNC_
                </div>
              </div>
              
              <div className="flex flex-col gap-4">
                {[
                  { name: "NeonSamurai", time: "02H:14M AGO", text: "Visuals are absolutely mind-blowing. The combat mechanics feel like a true next-gen experience. Highly recommended for any netrunner.", rating: "10.0" },
                  { name: "CyberJunkie99", time: "05H:30M AGO", text: "Performance drops in heavily populated megacities, but the storyline compensates for it. Solid build overall.", rating: "8.5" },
                  { name: "GlitchWlkr", time: "1D:04H AGO", text: "Waiting for the expansion patch. The base game is already a masterpiece of world-building. Don't skip the side quests.", rating: "9.2" }
                ].map((comment, i) => (
                  <div key={i} className="bg-[#1a0a2e]/60 backdrop-blur-md border-l-2 border-[#ff00ff] p-5 rounded-r-xl relative overflow-hidden group hover:bg-[#1a0a2e]/80 transition-all border-y border-r border-[#ffffff05]">
                    <div className="absolute top-0 right-0 w-32 h-32 bg-[#ff00ff]/5 rounded-full blur-3xl -mr-10 -mt-10 group-hover:bg-[#ff00ff]/20 transition-colors duration-500"></div>
                    <div className="flex justify-between items-start mb-3 relative z-10">
                      <div className="flex items-center gap-3">
                        <div className="w-9 h-9 bg-gradient-to-br from-[#ff00ff]/80 to-[#00b0f0]/80 rounded-sm flex items-center justify-center text-xs font-black text-white shadow-[0_0_15px_rgba(255,0,255,0.4)] border border-white/20" style={{ clipPath: 'polygon(50% 0%, 100% 25%, 100% 75%, 50% 100%, 0% 75%, 0% 25%)' }}>
                          {comment.name.substring(0,2).toUpperCase()}
                        </div>
                        <div>
                          <div className="text-white font-bold text-sm tracking-wide group-hover:text-transparent group-hover:bg-clip-text group-hover:bg-gradient-to-r group-hover:from-white group-hover:to-[#ff00ff] transition-all">{comment.name}</div>
                          <div className="text-[#00b0f0] text-[10px] uppercase font-mono mt-0.5">
                            {comment.time}
                          </div>
                        </div>
                      </div>
                      <div className="text-[#ff00ff] font-mono font-bold text-lg bg-[#ff00ff]/10 px-2.5 py-0.5 rounded border border-[#ff00ff]/30 drop-shadow-[0_0_5px_rgba(255,0,255,0.5)]">{comment.rating}</div>
                    </div>
                    <p className="text-slate-200 text-sm leading-relaxed relative z-10 font-medium">"{comment.text}"</p>
                  </div>
                ))}
              </div>
            </div>""")
        continue
        
    if in_replace_zone:
        # Check if we reached the end of the area we want to replace
        if line == "            )}":
            # Peek at the next few lines. The last part is `)}`. But we know similar games ends at `)}`.
            # Wait, the end is exactly `            )}` just before the closing flex container `          </div>`.
            pass
        if line == "          </div>":
            # Reached end
            in_replace_zone = False
            out_lines.append(line)
        continue
    
    out_lines.append(line)

new_content = "\n".join(out_lines)

with open(file_path, "w", encoding="utf-8") as f:
    f.write(new_content)

print(f"File updated. Zone replaced: {zone_found}")
