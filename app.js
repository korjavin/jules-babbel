document.addEventListener('DOMContentLoaded', () => {
    // --- DOM Elements ---
    const settingsBtn = document.getElementById('settings-btn');
    const settingsModal = document.getElementById('settings-modal');
    const settingsCloseBtn = document.getElementById('settings-close-btn');
    const topicSelector = document.getElementById('topic-selector');

    const generateBtn = document.getElementById('generate-btn');
    const hintBtn = document.getElementById('hint-btn');
    const loadingSpinner = document.getElementById('loading-spinner');
    const exerciseContent = document.getElementById('exercise-content');

    const englishHintEl = document.getElementById('english-hint');
    const answerArea = document.getElementById('answer-area');
    const answerPrompt = document.getElementById('answer-prompt');
    const constructedSentenceEl = document.getElementById('constructed-sentence');
    const scrambledWordsContainer = document.getElementById('scrambled-words-container');
    const feedbackArea = document.getElementById('feedback-area');
    const correctSentenceDisplay = document.getElementById('correct-sentence-display');
    const exerciseCounter = document.getElementById('exercise-counter');
    const emptyStateContainer = document.getElementById('empty-state-container');

    const statsMistakesEl = document.getElementById('stats-mistakes');
    const statsHintsEl = document.getElementById('stats-hints');

    // Topics management elements
    const topicsList = document.getElementById('topics-list');
    const addTopicBtn = document.getElementById('add-topic-btn');
    const addTopicForm = document.getElementById('add-topic-form');
    const newTopicName = document.getElementById('new-topic-name');
    const newTopicPrompt = document.getElementById('new-topic-prompt');
    const saveTopicBtn = document.getElementById('save-topic-btn');
    const cancelAddBtn = document.getElementById('cancel-add-btn');

    // Prompt editor elements
    const promptEditor = document.getElementById('prompt-editor');
    const currentTopicName = document.getElementById('current-topic-name');
    const promptTextarea = document.getElementById('prompt-textarea');
    const savePromptBtn = document.getElementById('save-prompt-btn');
    const cancelEditBtn = document.getElementById('cancel-edit-btn');
    
    // Version history elements
    const viewVersionsBtn = document.getElementById('view-versions-btn');
    const versionHistory = document.getElementById('version-history');
    const versionTopicName = document.getElementById('version-topic-name');
    const versionsList = document.getElementById('versions-list');
    const closeVersionsBtn = document.getElementById('close-versions-btn');

    // --- Application State ---
    let state = {
        currentTopicId: '',
        topics: [],
        exercises: [],
        currentExerciseIndex: 0,
        userSentence: [],
        isLocked: false,
        mistakes: 0,
        hintsUsed: 0,
        startTime: null,
        sessionTime: 0,
        isSessionComplete: false,
        editingTopicId: null
    };

    // --- Sample Data ---
    const sampleExercises = {
        "exercises": [
            {
                "conjunction_topic": "weil",
                "english_hint": "He is learning German because he wants to work in Germany.",
                "correct_german_sentence": "Er lernt Deutsch, weil er in Deutschland arbeiten will.",
                "scrambled_words": ["er", "in", "will", "arbeiten", "Deutschland", "lernt", "Deutsch,", "weil"]
            },
            {
                "conjunction_topic": "obwohl",
                "english_hint": "She is going for a walk, although it is raining.",
                "correct_german_sentence": "Sie geht spazieren, obwohl es regnet.",
                "scrambled_words": ["obwohl", "es", "Sie", "geht", "spazieren,", "regnet"]
            }
        ]
    };

    // --- Helper Functions ---
    function getHotkey(index) {
        if (index < 9) {
            return (index + 1).toString(); // 1-9
        } else {
            return String.fromCharCode(97 + index - 9); // a, b, c, etc.
        }
    }

    function isPunctuation(token) {
        return /^[^\p{L}\p{N}]+$/u.test(token);
    }

    function addPunctuationIfNeeded(exercise, userSentence) {
        const correctWordArray = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        
        while (userSentence.length < correctWordArray.length) {
            const nextToken = correctWordArray[userSentence.length];
            if (isPunctuation(nextToken)) {
                userSentence.push(nextToken);
            } else {
                break;
            }
        }
    }

    function updateStats() {
        statsMistakesEl.textContent = `Mistakes: ${state.mistakes}`;
        statsHintsEl.textContent = `Hints Used: ${state.hintsUsed}`;
    }

    // --- Exercise Rendering and Logic ---
    function renderExercise() {
        state.isLocked = false;
        state.userSentence = [];

        if (state.exercises.length === 0) {
            exerciseContent.classList.add('hidden');
            emptyStateContainer.classList.remove('hidden');
            exerciseCounter.classList.add('hidden');
            hintBtn.classList.add('hidden');
            return;
        }

        exerciseContent.classList.remove('hidden');
        emptyStateContainer.classList.add('hidden');
        exerciseCounter.classList.remove('hidden');
        hintBtn.classList.remove('hidden');

        const exercise = state.exercises[state.currentExerciseIndex];

        exerciseCounter.textContent = `${state.currentExerciseIndex + 1} / ${state.exercises.length}`;

        // Reset UI
        englishHintEl.textContent = exercise.english_hint;
        scrambledWordsContainer.innerHTML = '';
        constructedSentenceEl.innerHTML = '';
        answerPrompt.classList.remove('hidden');
        correctSentenceDisplay.textContent = '';

        // Tokenize the correct sentence to create word buttons, then shuffle them.
        const allTokens = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        const wordsToDisplay = allTokens.filter(token => !isPunctuation(token));
        for (let i = wordsToDisplay.length - 1; i > 0; i--) {
            const j = Math.floor(Math.random() * (i + 1));
            [wordsToDisplay[i], wordsToDisplay[j]] = [wordsToDisplay[j], wordsToDisplay[i]];
        }

        // Create and display word buttons with hotkeys
        wordsToDisplay.forEach((word, index) => {
            const button = document.createElement('button');
            const hotkey = getHotkey(index);
            
            const hotkeySpan = document.createElement('span');
            hotkeySpan.textContent = hotkey;
            hotkeySpan.className = 'hotkey-indicator';
            
            const wordSpan = document.createElement('span');
            wordSpan.textContent = word;
            
            button.appendChild(hotkeySpan);
            button.appendChild(wordSpan);
            
            button.className = 'btn-word px-4 py-2 rounded-md font-medium';
            button.dataset.hotkey = hotkey;
            button.dataset.word = word;
            
            button.addEventListener('click', () => handleWordClick(word, button));
            
            scrambledWordsContainer.appendChild(button);
        });
    }

    function handleWordClick(word, button) {
        if (state.isLocked) return;

        const exercise = state.exercises[state.currentExerciseIndex];
        
        state.userSentence.push(word);
        addPunctuationIfNeeded(exercise, state.userSentence);

        // Hide the clicked button
        button.classList.add('hidden');

        // Update constructed sentence display
        constructedSentenceEl.innerHTML = '';
        answerPrompt.classList.add('hidden');
        
        state.userSentence.forEach(word => {
            const span = document.createElement('span');
            span.textContent = word;
            span.className = 'px-2 py-1 bg-gray-100 rounded mr-1';
            constructedSentenceEl.appendChild(span);
        });

        // Check if sentence is complete
        const correctWordArray = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        
        if (state.userSentence.length === correctWordArray.length) {
            handleSentenceCompletion(exercise, correctWordArray);
        }
    }

    function handleSentenceCompletion(exercise, correctWordArray) {
        state.isLocked = true;
        const isCorrect = state.userSentence.join(' ') === correctWordArray.join(' ');
        
        if (isCorrect) {
            correctSentenceDisplay.textContent = `Correct! "${exercise.correct_german_sentence}"`;
            
            setTimeout(() => {
                if (state.currentExerciseIndex < state.exercises.length - 1) {
                    state.currentExerciseIndex++;
                    renderExercise();
                } else {
                    showStatisticsPage();
                }
            }, 2000);
        } else {
            state.mistakes++;
            updateStats();
            
            // Show incorrect feedback
            const wrongWords = scrambledWordsContainer.querySelectorAll('.btn-word.hidden');
            wrongWords.forEach(btn => {
                btn.classList.add('incorrect-answer-feedback');
                setTimeout(() => {
                    btn.classList.remove('incorrect-answer-feedback');
                }, 500);
            });
            
            // Reset for another try
            setTimeout(() => {
                state.userSentence = [];
                renderExercise();
            }, 1500);
        }
    }

    function handleHintClick() {
        if (state.isLocked || state.exercises.length === 0) return;

        const exercise = state.exercises[state.currentExerciseIndex];
        const correctWordArray = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        const nonPunctuationWords = correctWordArray.filter(token => !isPunctuation(token));
        
        if (state.userSentence.length < nonPunctuationWords.length) {
            const nextCorrectWord = nonPunctuationWords[state.userSentence.length];
            const availableButtons = scrambledWordsContainer.querySelectorAll('.btn-word:not(.hidden)');
            
            for (const button of availableButtons) {
                if (button.dataset.word === nextCorrectWord) {
                    button.classList.add('hint-word');
                    state.hintsUsed++;
                    updateStats();
                    
                    setTimeout(() => {
                        button.classList.remove('hint-word');
                    }, 2000);
                    break;
                }
            }
        }
    }

    function showStatisticsPage() {
        state.isSessionComplete = true;
        const endTime = Date.now();
        state.sessionTime = Math.floor((endTime - state.startTime) / 1000);
        
        const accuracy = state.exercises.length > 0 ? 
            Math.round(((state.exercises.length - state.mistakes) / state.exercises.length) * 100) : 0;
        const avgTimePerExercise = state.exercises.length > 0 ? 
            (state.sessionTime / state.exercises.length).toFixed(1) : 0;

        // Create statistics display
        const statsContainer = document.createElement('div');
        statsContainer.id = 'statistics-container';
        statsContainer.className = 'card rounded-lg p-8 text-center';
        
        statsContainer.innerHTML = `
            <h2 class="text-3xl font-bold text-gray-800 mb-6">Session Complete! 🎉</h2>
            <div class="grid grid-cols-2 md:grid-cols-3 gap-6 mb-8">
                <div class="text-center">
                    <div class="text-2xl font-bold text-[#A58D78]">${state.exercises.length}</div>
                    <div class="text-gray-600">Exercises Completed</div>
                </div>
                <div class="text-center">
                    <div class="text-2xl font-bold text-[#A58D78]">${state.mistakes}</div>
                    <div class="text-gray-600">Total Mistakes</div>
                </div>
                <div class="text-center">
                    <div class="text-2xl font-bold text-[#A58D78]">${state.hintsUsed}</div>
                    <div class="text-gray-600">Hints Used</div>
                </div>
                <div class="text-center">
                    <div class="text-2xl font-bold text-[#A58D78]">${accuracy}%</div>
                    <div class="text-gray-600">Accuracy</div>
                </div>
                <div class="text-center">
                    <div class="text-2xl font-bold text-[#A58D78]">${state.sessionTime}s</div>
                    <div class="text-gray-600">Total Time</div>
                </div>
                <div class="text-center">
                    <div class="text-2xl font-bold text-[#A58D78]">${avgTimePerExercise}s</div>
                    <div class="text-gray-600">Avg per Exercise</div>
                </div>
            </div>
            <div class="flex flex-col sm:flex-row gap-4 justify-center">
                <button id="new-session-btn" class="btn-primary px-6 py-3 rounded-lg font-semibold text-lg">
                    Start New Session
                </button>
                <button id="same-exercises-btn" class="btn-primary px-6 py-3 rounded-lg font-semibold text-lg">
                    Practice Same Exercises
                </button>
            </div>
        `;

        // Replace exercise content with statistics
        document.getElementById('exercise-container').classList.add('hidden');
        document.querySelector('.text-center').classList.add('hidden');
        document.querySelector('main .max-w-3xl').appendChild(statsContainer);

        // Add event listeners for the buttons
        document.getElementById('new-session-btn').addEventListener('click', resetForNewSession);
        document.getElementById('same-exercises-btn').addEventListener('click', resetForSameExercises);
    }

    function resetForNewSession() {
        const statsContainer = document.getElementById('statistics-container');
        if (statsContainer) {
            statsContainer.remove();
        }
        
        document.getElementById('exercise-container').classList.remove('hidden');
        document.querySelector('.text-center').classList.remove('hidden');
        
        state.currentExerciseIndex = 0;
        state.mistakes = 0;
        state.hintsUsed = 0;
        state.sessionTime = 0;
        state.isSessionComplete = false;
        state.startTime = null;
        state.exercises = [];
        
        updateStats();
        renderExercise(); // Show empty state
    }

    function resetForSameExercises() {
        const statsContainer = document.getElementById('statistics-container');
        if (statsContainer) {
            statsContainer.remove();
        }
        
        document.getElementById('exercise-container').classList.remove('hidden');
        document.querySelector('.text-center').classList.remove('hidden');
        
        state.currentExerciseIndex = 0;
        state.mistakes = 0;
        state.hintsUsed = 0;
        state.sessionTime = 0;
        state.isSessionComplete = false;
        state.startTime = Date.now();
        
        updateStats();
        renderExercise();
    }

    function handleKeyPress(event) {
        if (state.isLocked) return;
        
        const key = event.key.toLowerCase();
        const wordButtons = scrambledWordsContainer.querySelectorAll('.btn-word:not(.hidden)');
        
        for (const button of wordButtons) {
            if (button.dataset.hotkey === key) {
                button.click();
                break;
            }
        }
    }

    // --- Topics API Functions ---
    async function loadTopics() {
        try {
            const response = await fetch('/api/topics');
            if (!response.ok) throw new Error('Failed to load topics');
            
            const data = await response.json();
            state.topics = data.topics || [];
            
            populateTopicSelector();
            renderTopicsList();
            
            // Load selected topic from localStorage or use first available
            const savedTopicId = localStorage.getItem('selectedTopicId');
            if (savedTopicId && state.topics.find(t => t.id === savedTopicId)) {
                state.currentTopicId = savedTopicId;
            } else if (state.topics.length > 0) {
                state.currentTopicId = state.topics[0].id;
            }
            
            topicSelector.value = state.currentTopicId;
            
        } catch (error) {
            console.error('Error loading topics:', error);
            alert('Failed to load topics. Please refresh the page.');
        }
    }

    function populateTopicSelector() {
        topicSelector.innerHTML = '';
        
        if (state.topics.length === 0) {
            const option = document.createElement('option');
            option.value = '';
            option.textContent = 'No topics available';
            topicSelector.appendChild(option);
            return;
        }
        
        state.topics.forEach(topic => {
            const option = document.createElement('option');
            option.value = topic.id;
            option.textContent = topic.name;
            topicSelector.appendChild(option);
        });
    }

    function renderTopicsList() {
        topicsList.innerHTML = '';
        
        state.topics.forEach(topic => {
            const topicDiv = document.createElement('div');
            topicDiv.className = 'flex justify-between items-center p-3 border rounded-md bg-gray-50';
            
            topicDiv.innerHTML = `
                <div>
                    <div class="font-medium">${topic.name}</div>
                    <div class="text-sm text-gray-500">Created: ${new Date(topic.created_at).toLocaleDateString()}</div>
                </div>
                <div class="flex space-x-2">
                    <button class="edit-topic-btn text-blue-600 hover:text-blue-800 text-sm" data-topic-id="${topic.id}">Edit</button>
                    <button class="delete-topic-btn text-red-600 hover:text-red-800 text-sm" data-topic-id="${topic.id}">Delete</button>
                </div>
            `;
            
            topicsList.appendChild(topicDiv);
        });
        
        // Add event listeners for edit and delete buttons
        topicsList.querySelectorAll('.edit-topic-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const topicId = e.target.dataset.topicId;
                showPromptEditor(topicId);
            });
        });
        
        topicsList.querySelectorAll('.delete-topic-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const topicId = e.target.dataset.topicId;
                deleteTopic(topicId);
            });
        });
    }

    async function createTopic(name, prompt) {
        try {
            const response = await fetch('/api/topics', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name, prompt })
            });
            
            if (!response.ok) throw new Error('Failed to create topic');
            
            await loadTopics(); // Refresh the topics list
            hideAddTopicForm();
            
        } catch (error) {
            console.error('Error creating topic:', error);
            alert('Failed to create topic. Please try again.');
        }
    }

    async function deleteTopic(topicId) {
        if (!confirm('Are you sure you want to delete this topic? This action cannot be undone.')) {
            return;
        }
        
        try {
            const response = await fetch(`/api/topics/${topicId}`, {
                method: 'DELETE'
            });
            
            if (!response.ok) throw new Error('Failed to delete topic');
            
            // If this was the selected topic, clear selection
            if (state.currentTopicId === topicId) {
                state.currentTopicId = '';
                localStorage.removeItem('selectedTopicId');
            }
            
            await loadTopics(); // Refresh the topics list
            
        } catch (error) {
            console.error('Error deleting topic:', error);
            alert('Failed to delete topic. Please try again.');
        }
    }

    async function updateTopicPrompt(topicId, prompt) {
        try {
            const response = await fetch(`/api/topics/${topicId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ prompt })
            });
            
            if (!response.ok) throw new Error('Failed to update prompt');
            
            await loadTopics(); // Refresh the topics list
            hidePromptEditor();
            
        } catch (error) {
            console.error('Error updating prompt:', error);
            alert('Failed to update prompt. Please try again.');
        }
    }

    async function fetchExercises() {
        if (!state.currentTopicId) {
            alert('Please select a topic first.');
            return;
        }

        loadingSpinner.classList.remove('hidden');
        exerciseContent.classList.add('hidden');
        generateBtn.disabled = true;

        try {
            const response = await fetch('/api/generate', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    topic_id: state.currentTopicId
                })
            });

            if (!response.ok) {
                const errorData = await response.json();
                throw new Error(`API Error: ${response.statusText} - ${errorData.error.message}`);
            }

            const data = await response.json();
            const content = JSON.parse(data.choices[0].message.content);

            if (content.exercises && content.exercises.length > 0) {
                state.exercises = content.exercises;
                state.currentExerciseIndex = 0;
                state.mistakes = 0;
                state.hintsUsed = 0;
                state.sessionTime = 0;
                state.isSessionComplete = false;
                state.startTime = Date.now();
                updateStats();
                renderExercise();
            } else {
                throw new Error('Invalid data structure received from API.');
            }

        } catch (error) {
            console.error('Error fetching exercises:', error);
            alert(`Failed to fetch new exercises. Please check your API key and network connection. \nError: ${error.message}`);
            renderExercise();
        } finally {
            loadingSpinner.classList.add('hidden');
            generateBtn.disabled = false;
        }
    }

    // --- UI Helper Functions ---
    function showAddTopicForm() {
        addTopicForm.classList.remove('hidden');
        newTopicName.value = '';
        newTopicPrompt.value = '';
        newTopicName.focus();
    }

    function hideAddTopicForm() {
        addTopicForm.classList.add('hidden');
    }

    function showPromptEditor(topicId) {
        const topic = state.topics.find(t => t.id === topicId);
        if (!topic) return;
        
        state.editingTopicId = topicId;
        currentTopicName.textContent = topic.name;
        promptTextarea.value = topic.prompt;
        promptEditor.classList.remove('hidden');
        versionHistory.classList.add('hidden');
    }

    function hidePromptEditor() {
        promptEditor.classList.add('hidden');
        state.editingTopicId = null;
    }

    async function showVersionHistory(topicId) {
        try {
            const response = await fetch(`/api/versions/${topicId}`);
            if (!response.ok) throw new Error('Failed to load versions');
            
            const data = await response.json();
            const versions = data.versions || [];
            
            const topic = state.topics.find(t => t.id === topicId);
            versionTopicName.textContent = topic ? topic.name : 'Unknown Topic';
            
            versionsList.innerHTML = '';
            
            versions.reverse().forEach(version => {
                const versionDiv = document.createElement('div');
                versionDiv.className = 'flex justify-between items-center p-2 border rounded text-sm';
                
                versionDiv.innerHTML = `
                    <div>
                        <div class="font-medium">Version ${version.version}</div>
                        <div class="text-gray-500">${new Date(version.created_at).toLocaleString()}</div>
                    </div>
                    <button class="restore-version-btn text-blue-600 hover:text-blue-800" 
                            data-topic-id="${topicId}" data-version-id="${version.id}">
                        Restore
                    </button>
                `;
                
                versionsList.appendChild(versionDiv);
            });
            
            // Add event listeners for restore buttons
            versionsList.querySelectorAll('.restore-version-btn').forEach(btn => {
                btn.addEventListener('click', async (e) => {
                    const topicId = e.target.dataset.topicId;
                    const versionId = e.target.dataset.versionId;
                    await restoreVersion(topicId, versionId);
                });
            });
            
            promptEditor.classList.add('hidden');
            versionHistory.classList.remove('hidden');
            
        } catch (error) {
            console.error('Error loading version history:', error);
            alert('Failed to load version history.');
        }
    }

    async function restoreVersion(topicId, versionId) {
        if (!confirm('Are you sure you want to restore this version? This will create a new version with this content.')) {
            return;
        }
        
        try {
            const response = await fetch(`/api/versions/${topicId}/restore/${versionId}`, {
                method: 'POST'
            });
            
            if (!response.ok) throw new Error('Failed to restore version');
            
            await loadTopics(); // Refresh topics
            versionHistory.classList.add('hidden');
            alert('Version restored successfully!');
            
        } catch (error) {
            console.error('Error restoring version:', error);
            alert('Failed to restore version.');
        }
    }

    // --- Event Listeners ---
    settingsBtn.addEventListener('click', () => {
        loadTopics(); // Refresh topics when opening settings
        settingsModal.classList.remove('hidden');
    });

    settingsCloseBtn.addEventListener('click', () => {
        settingsModal.classList.add('hidden');
        hideAddTopicForm();
        hidePromptEditor();
        versionHistory.classList.add('hidden');
    });

    topicSelector.addEventListener('change', (e) => {
        state.currentTopicId = e.target.value;
        localStorage.setItem('selectedTopicId', state.currentTopicId);
    });

    addTopicBtn.addEventListener('click', showAddTopicForm);
    cancelAddBtn.addEventListener('click', hideAddTopicForm);

    saveTopicBtn.addEventListener('click', () => {
        const name = newTopicName.value.trim();
        const prompt = newTopicPrompt.value.trim();
        
        if (!name || !prompt) {
            alert('Please provide both a name and a prompt.');
            return;
        }
        
        createTopic(name, prompt);
    });

    cancelEditBtn.addEventListener('click', hidePromptEditor);

    savePromptBtn.addEventListener('click', () => {
        const prompt = promptTextarea.value.trim();
        
        if (!prompt) {
            alert('Prompt cannot be empty.');
            return;
        }
        
        updateTopicPrompt(state.editingTopicId, prompt);
    });

    viewVersionsBtn.addEventListener('click', () => {
        if (state.editingTopicId) {
            showVersionHistory(state.editingTopicId);
        }
    });

    closeVersionsBtn.addEventListener('click', () => {
        versionHistory.classList.add('hidden');
        promptEditor.classList.remove('hidden');
    });

    generateBtn.addEventListener('click', fetchExercises);
    hintBtn.addEventListener('click', handleHintClick);
    document.addEventListener('keydown', handleKeyPress);

    // --- Initialization ---
    function init() {
        loadTopics();
        
        // Start with sample exercises for testing
        state.exercises = sampleExercises.exercises;
        state.currentExerciseIndex = 0;
        state.mistakes = 0;
        state.hintsUsed = 0;
        state.sessionTime = 0;
        state.isSessionComplete = false;
        state.startTime = Date.now();
        
        updateStats();
        renderExercise();
    }

    init();
});